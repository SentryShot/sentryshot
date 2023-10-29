```
apt install qemu binfmt-support qemu-user-static

docker run --rm --privileged multiarch/qemu-user-static --reset -p yes

docker run -it \
	-v ./out:/out \
	-v ./shell.nix:/shell.nix \
	-v $(pwd)/../patches/:/patches \
	-v $(pwd)/../src-deps.json:/src-deps.json \
	--platform linux/arm64/v8 \
	nixos/nix:2.15.2-arm64 bash

nix-shell --option filter-syscalls false

cp $TFLITELIB/libtensorflowlite_c.so /out/
```
