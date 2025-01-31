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

var addr string


func run(service roverlib.Service, configuration *roverlib.ServiceConfiguration) error {
	if configuration == nil {
		return fmt.Errorf("configuration cannot be accessed")
	}
	
	
	actuatorOutput := service.GetWriteStream("decision")
	
	addr, err := configuration.GetStringSafe("controller-address")
	if err != nil {
		return err
	}
	exec.Command("bluetoothctl", "trust", addr)
	time.Sleep(2 * time.Second)

	exec.Command("bluetoothctl", "scan", "on")

	exec.Command("bluetoothctl", "power", "on")
	exec.Command("bluetoothctl", "agent", "on")
	exec.Command("bluetoothctl", "default-agent")
	

	pairCmd := exec.Command("bluetoothctl", "pair", addr)
	if output, err := pairCmd.CombinedOutput(); err != nil {
		log.Info().Msgf("Failed to pair device: %s\nOutput: %s\n", err, string(output))
	} else {
		log.Info().Msgf("Pairing output: %s\n", string(output))
	}
	time.Sleep(2 * time.Second)

	log.Info().Msgf("Connecting to the device...")
	connectCmd := exec.Command("bluetoothctl", "connect", addr)
	if output, err := connectCmd.CombinedOutput(); err != nil {
		log.Info().Msgf("Failed to connect to device: %s\nOutput: %s\n", err, string(output))
		return err
	} else {
		log.Info().Msgf("Connection output: %s\n", string(output))
	}


	time.Sleep(2 * time.Second)

	devices, err := evdev.ListInputDevices("/dev/input/event*")
	if err != nil {
		log.Fatal().Msgf("Failed to list input devices: %v", err)
	}

	acc := 0
	vel := 0.0
	cc := false
	stop := false
	steer := 0.0

	var codeForward uint16
	var codeBackward uint16
	var codeCC uint16 //cruise-control
	var codeEB uint16 //emergency-brake
	var codeSteer uint16

	var controller *evdev.InputDevice

	// Find the controller
	for _, dev := range devices {
		if strings.Contains(strings.ToLower(dev.Name), "dualsense") {
			controller = dev
			log.Info().Msgf("PlayStation 5 controller detected")

			codeForward = 313
			codeBackward = 312
			codeCC = 307
			codeEB = 304
			codeSteer = 0

			break
		} else if strings.Contains(strings.ToLower(dev.Name), "joy-con (l)") {
			controller = dev
			log.Info().Msgf("Left Joy-Con controller detected")
			
			// NOT IMPLEMENTED
			// codeForward =
			// codeBackward =
			// codeCC =
			// codeEB =
			// codeSteer =

			break
		} else if strings.Contains(strings.ToLower(dev.Name), "rvl-cnt") {
			controller = dev
			log.Info().Msgf("Wii Remote controller detected")
			
			// NOT IMPLEMENTED
			// codeForward =
			// codeBackward =
			// codeCC =
			// codeEB =
			// codeSteer =

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
								cc = !cc
								log.Info().Msgf("CC toggled to: %t", cc)
							}
							
						case codeEB: // em-brake
							stop = ev.Value == 1
							log.Info().Msgf("EMBRAKE: %t", stop)
							cc = false
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
		if !cc {
			vel += float64(acc) / 100
			
			//when no acceleration is present (forward or backward), velocity starts approaching 0. 
			if(acc == 0){
				vel *= 0.6
				
				// stop endless approaching of 0
				if(vel < 0.01 && vel > 0.01){
					vel = 0
				}
			}
			
			// max speed
			if(vel < -0.3){
				vel = -0.3
			} else if(vel > 0.3){
				vel = 0.3
			}
		}

		if(stop){
			vel = 0.0
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

	exec.Command("bluetoothctl", "disconnect", addr)
	
	return nil
}

// This is just a wrapper to run the user program
// it is not recommended to put any other logic here
func main() {
	roverlib.Run(run, onTerminate)
}
