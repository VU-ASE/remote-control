package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	roverlib "github.com/VU-ASE/roverlib-go/src"
	pb_outputs "github.com/VU-ASE/rovercom/packages/go/outputs"

	evdev "github.com/gvalkov/golang-evdev"
	"github.com/rs/zerolog/log"
)

var controllerAddress string

type Controller struct{
	codeForward int
	codeBackward int
	codeCC int
	codeEB int
	codeSteer int
	controllerCode string
	bluetoothName string
	name string
}

var supportedControllers = []Controller{
	{313, 312, 307, 304, 0, "ps5", "dualsense", "Playstation 5"}, 
	
	// Add any other controllers...

} 



func run(service roverlib.Service, configuration *roverlib.ServiceConfiguration) error {

	if(configuration == nil){
		log.Error().Msgf("no config")
		return fmt.Errorf("configuration cannot be accessed")
	}

	
	actuatorOutput := service.GetWriteStream("decision")

	controllerAddress, err := configuration.GetStringSafe("controller-address")
	if(err != nil){
		return err
	}

	controllerType, err := configuration.GetStringSafe("controller-type")
	if(err != nil){
		return err
	}

	maxSpeed, err := configuration.GetFloatSafe("max-speed")
	if(err != nil){
		return err
	}
	// prematurely trust the given mac-address, to ensure no problems while pairing
	exec.Command("bluetoothctl", "trust", controllerAddress)
	time.Sleep(2 * time.Second)

	// start scanning for devices
	output := exec.Command("bluetoothctl", "scan", "on")
	_, err = output.CombinedOutput()
	if(err != nil){
		log.Error().Msgf("Error during scanning: %s", err)
		return err
	}

	// then turn on bluetooth adapter
	output = exec.Command("bluetoothctl", "power", "on")
	_, err = output.CombinedOutput()
	if(err != nil){
		log.Error().Msgf("Error while turning on bluetooth adapter: %s", err)
		return err
	}

	// activate the agent to handle pairing requests
	output = exec.Command("bluetoothctl", "agent", "on")
	_, err = output.CombinedOutput()
	if(err != nil){
		log.Error().Msgf("Error while activating agent: %s", err)
		return err
	}

	// then set the current agent as the default for managing pairings
	output = exec.Command("bluetoothctl", "default-agent")
	_, err = output.CombinedOutput()
	if(err != nil){
		log.Error().Msgf("Error while setting default agent: %s", err)
		return err
	}
	
	// pair to the device, wait before connecting to ensure successful pairing
	pairCmd := exec.Command("bluetoothctl", "pair", controllerAddress)
	if output, err := pairCmd.CombinedOutput(); err != nil {
		log.Info().Msgf("Failed to pair device: %s\nOutput: %s\n", err, string(output))
	} else {
		log.Info().Msgf("Pairing output: %s\n", string(output))
	}
	time.Sleep(2 * time.Second)

	log.Info().Msgf("Connecting to the device...")
	connectCmd := exec.Command("bluetoothctl", "connect", controllerAddress)
	if output, err := connectCmd.CombinedOutput(); err != nil {
		log.Info().Msgf("Failed to connect to device: %s\nOutput: %s\n", err, string(output))
		return err
	} else {
		log.Info().Msgf("Connection output: %s\n", string(output))
	}

	// sleep to ensure device is connected before accessing it
	time.Sleep(2 * time.Second)

	devices, err := evdev.ListInputDevices("/dev/input/event*")
	if err != nil {
		log.Fatal().Msgf("Failed to list input devices: %v", err)
	}

	acc := 0
	vel := 0.0
	cruiseControl := false
	stop := false
	steer := 0.0

	// these store the codes that the evdev library binds to the used buttons
	// stored in a var for easy scalability when adding additional controller support
	var codeForward uint16
	var codeBackward uint16
	var codeCC uint16 //cruise-control
	var codeEB uint16 //emergency-brake
	var codeSteer uint16
	var bluetoothName string

	found := false
	var controller *evdev.InputDevice
	for _, controller := range supportedControllers{
		if(controllerType == controller.controllerCode){
			codeForward = uint16(controller.codeForward)
			codeBackward = uint16(controller.codeBackward)
			codeCC = uint16(controller.codeCC)
			codeEB = uint16(controller.codeEB)
			codeSteer = uint16(controller.codeSteer)
			bluetoothName = controller.bluetoothName
			found = true
			break
		}	
	}
	if(!found){
		log.Error().Msgf("Unknown controller type or not supported: %s", controllerType)
		log.Info().Msgf("List of valid controllers: ")
		for _, controller := range supportedControllers{
			log.Info().Msgf("%s with \"controller-type\": %s",controller.name, controller.controllerCode)
		}
		return err
	}


	// find the controller, which should be connected now
	for _, dev := range devices {
		if strings.Contains(strings.ToLower(dev.Name), bluetoothName) {
			controller = dev
			break
		} 
	}

	if controller == nil {
		log.Fatal().Msgf("No game controller found.")
		return err
	}

	log.Info().Msgf("Using controller: %s\n", controller.Name)

	f, err := os.Open(controller.Fn)
	if err != nil {
		log.Fatal().Msgf("Failed to open device: %v", err)
	}
	defer f.Close()

	// ticker function for updating speed, must update periodically
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	//blocking function that reads input and changes important variables
	go func() {
		for {
			events, err := controller.Read()
			if err != nil {
				log.Fatal().Msgf("Failed to read events: %v", err)
			}

			for _, ev := range events {
				switch ev.Type {
				case evdev.EV_KEY:
					switch ev.Code {
						case codeForward: 
							if ev.Value == 1 {
								acc++
							} else {
								acc--
							}
							log.Info().Msgf("acc: %d", acc)

						case codeBackward:
							if ev.Value == 1 {
								acc--
							} else {
								acc++
							}
							log.Info().Msgf("acc: %d", acc)

						case codeCC: // cruise-control
							if ev.Value == 1 {
								cruiseControl = !cruiseControl
								log.Info().Msgf("CC toggled to: %t", cruiseControl)
							}
							
						case codeEB: // emergency-brake
							stop = ev.Value == 1
							log.Info().Msgf("EMBRAKE: %t", stop)
							cruiseControl = false
					}
				case evdev.EV_ABS: // Axis movement
					if ev.Code == codeSteer {
	
						//neutral sits between 127 and 128, otherwise spamming this message
						if ev.Value < 127 || ev.Value > 128 {
							log.Info().Msgf("Axis %d moved to %d\n", ev.Code, ev.Value)
						}

						steer = float64(ev.Value)/128 - 1
					}
				}
			}
		}
	}()


	for range ticker.C{

		//don't update vel if cruise-control is on
		if(stop){
			vel = 0.0
		} else if !cruiseControl {
			vel += float64(acc) / 100
			//when no acceleration is present (forward or backward), velocity starts approaching 0. 
			if(acc == 0){
				vel *= 0.6
			}			
		}
		
		// stop endless approaching of 0
		if(vel < 0.01 && vel > 0.01){
			vel = 0
		} else if(vel < -maxSpeed){
			vel = -maxSpeed
		} else if(vel > maxSpeed){
			vel = maxSpeed
		}
		
		log.Info().Msgf("SPEED: %f. STEER: %f.", vel, steer)

		err = actuatorOutput.Write(
			&pb_outputs.SensorOutput{
				SensorId:  2,
				Timestamp: uint64(time.Now().UnixMilli()),
				SensorOutput: &pb_outputs.SensorOutput_ControllerOutput{
					ControllerOutput: &pb_outputs.ControllerOutput{
						SteeringAngle: float32(steer),
						LeftThrottle:  float32(vel),
						RightThrottle: float32(vel),
						FrontLights:   false,
					},
				},
			},
		)
		// Send it for the actuator (and others) to use
		if err != nil {
			log.Err(err).Msg("Failed to send controller output")
			continue
		}
	}

	return err
}



// This function gets called when roverd wants to terminate the service
func onTerminate(sig os.Signal) error {
	log.Info().Str("signal", sig.String()).Msg("Terminating service")
	exec.Command("bluetoothctl", "disconnect", controllerAddress)
	return nil
}

// This is just a wrapper to run the user program
// it is not recommended to put any other logic here
func main() {
	roverlib.Run(run, onTerminate)
}
