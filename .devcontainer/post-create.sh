#!/bin/sh
set -e

echo "deb https://packages.cloud.google.com/apt coral-edgetpu-stable main" | sudo tee /etc/apt/sources.list.d/coral-edgetpu.list

curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -

sudo apt update
sudo apt install -y libavcodec-dev libavutil-dev libedgetpu-dev libusb-1.0-0-dev
