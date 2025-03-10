# Usage

## Output

`remote-control` outputs the `decision` stream (same as `controller`), which is later read and interpreted by the `actuator`. See an example of encoding output below:

```
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
```

`SensorOutput_ControllerOutput` contains a `ControllerOutput` object, composed of the following fields:

1. `SteeringAngle`, a `float32` value between -1 (left) and 1 (right)
2. `LeftThrottle`, a `float32` value between -1 (full reverse) and 1 (full forward)
3. `RightThrottle`, a `float32` value between -1 (full reverse) and 1 (full forward)
4. `FrontLights`, a `boolean` value previously used to turn on the front lights in the dark, currently not used

For seeing an example of using `decision` stream, please look at `actuator documentation`

## Configuration

1. **controller-address** - this is the mac address of the bluetooth controller you will be using. Currently contains a hard-coded value for the PS5 controller you can find in the lab.
2. **controller-type** - a string label of the controller, needs to match with an external identifier of the controller you wish to connect
3. **max-speed** - puts a limit on how fast the rover can go on full throttle