# OLED controller

This directory includes a program to change the content of the OLED screens accompanied with certain keyboards, such as
the Lily58. It currently only looks for Lily58, and requires specific changes to the
[keymap](../keyboards/lily58/keymaps/albhen/keymap.c) to make it programmable, but the code can be adapted to support
other keyboards and devices with OLED screens.

In addition to proper support in the firmware, a custom [`glcdfont.c`](../keyboards/lily58/keymaps/albhen/glcdfont.c)
file is needed, to correctly show icons used for the bars, weather condition, fan, etc.

The program comes pre-programmed with three different views (called tags), which can be shown on the two OLED screens.
Events from the device can be sent to switch between the different tags. By default, USER00-USER05 can be mapped to
keys control which tag is shown on which screen. See [`keymap.c`](../keyboards/lily58/keymaps/albhen/keymap.c) for more
information.

## System status integration

Shows bar graphs representing the current utilization of the system.
* CPU% - Total CPU utilization for all cores and hyperthreads.
* Mem% - Memory utilization.
* Swap - Swap (page file) utilization.
* Disk - Disk I/O utilization.

On Linux, the flag `-sysstat-disk` can be specified to select for which harddisk to show utilization.

## GMail integration

Shows the number of unread messages for a certain label. This can be set up in multiple ways, but for a personal GMail
account, go to https://console.developers.google.com, create a new project, enable the GMail API, and then under
credentials create a new OAuth 2.0 Client ID. Download the JSON credentials file for that Client ID, and specify the
path to it using the `-gmail-credentials` flag to the OLED controller program. The OLED controller program will output
an URL that you need to visit, and after completing authentication, the website will show a token that needs to be
input to the OLED controller. Once this has been done once, the credentials will be cached, and the operation doesn't
need to be performed again (though you still need to specify the path to the downloaded credentials file).

## OpenWeatherMap integration

Shows the current temperature and weather condition in a specified location. An account needs to be created at
https://openweathermap.org, where you will get a personal API key that needs to be passed to the program, together with
the desired location for which to get the current weather. The location should be specified in the format
`<city>,<country>`, e.g. "Los Angeles,US".

## NVIDIA integration

Shows bar graphs representing the current utilization of the graphic card, as well as the current temperature.
* GPU% - GPU utilization.
* Mem% - Memory utilization.
* PCIe - PCIe bus utilization.
* Fan - Intended fan speed.

The temperature is in Celsius by default. This can be changed with the `-temperature-unit` flag.

NVML, NVIDIA Management Library, is used to gather status from the graphic card. A shared library needs to be installed
locally for this to work. On Linux, the shared library is called "libnvidia-ml.so", which probably comes together with
the NVIDIA drivers. On Windows the library is called "nvml.dll", and can be found in the CUDA toolkit.

## License

Copyright 2020 Albert "Drauthius" Diserholt. All rights reserved.
Licensed under the MIT License.
