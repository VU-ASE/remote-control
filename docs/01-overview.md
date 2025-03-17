# Overview

This is a service that allows you to directly control the rover with a bluetooth remote controller. `remote-control` does not take any input streams and produces a `decision` output stream, which can later be taken by the `actuator` to convert your commands into the actual movement. Out of the box, this is currently only compatible with [PlayStation5 Controller](https://www.playstation.com/en-us/support/hardware/pair-dualsense-controller-bluetooth/).
