# Installation

- [Docker Bundle Install](#docker-bundle)
- [Docker Compose Install](#docker-compose)
- [Bare Metal Install](#bare-metal-installation)
- [Webserver](#web-server)
- [Before continuing](#before-continuing)

<br>

## Docker Bundle Install
##### Note: This is the quick and dirty way to use Docker.

The Docker bundle can be installed using a single command.

```
docker run -it \
	--env TZ=America/New_York \
	--shm-size=500m \
	-v /home/_nvr/os-nvr/configs \
	-v /home/_nvr/os-nvr/storage \
	-p 2000:2000 \
	osnvr/os-nvr_bundle:latest
```

App will be served on `https://ip:2000`

[Coral EdgeTPU](https://www.coral.ai/products/accelerator) is supported, if you own a device add `--device=/dev/bus/usb \`

## Docker Compose Install

[Docker Compose](https://gitlab.com/osnvr/os-nvr_docker)

<br>

## Bare Metal Install

#### Install dependencies

- [Golang](https://golang.org/doc/install) 1.16+
- [ffmpeg](https://ffmpeg.org/download.html) 4.3+
- `git sed which`

#### Create an unprivileged user named `_nvr`

    sudo useradd -m -s /sbin/nologin _nvr

#### Go to `_nvr` home and clone repository.

    cd /home/_nvr/
    sudo -u _nvr git clone --branch master https://gitlab.com/osnvr/os-nvr.git
    cd ./os-nvr



#### Download Golang dependencies.
	
	sudo -u _nvr go mod download


#### Run service creation script

	sudo ./utils/services/systemd.sh \
		--name nvr \
		--goBin /usr/bin/go \
		--env /home/_nvr/os-nvr/configs/env.yaml \
		--homeDir /home/_nvr/os-nvr


#### Restart service to generate `env.yaml`

	sudo systemctl restart nvr


#### Enable the authentication addon in `env.yaml`

	sudo nano /home/_nvr/os-nvr/configs/env.yaml

```
before:
    #- nvr/addons/auth/none
after:
    - nvr/addons/auth/none
```


#### Restart service again.

	sudo systemctl restart nvr

The app should now be running on port `2020`

Continue to the next section if you don't have a web server already.

<br>

## Web server

This is included in the Docker bundle.

A web server is required for TLS, Websockets and HTTP/2. We will use Caddy but any HTTP/2 supported web server will do. [Install Caddy](https://caddyserver.com/docs/install)


Caddy is configured using a "[Caddyfile](https://caddyserver.com/docs/caddyfile)" default location is `/etc/caddy/Caddyfile`

### Enabling TLS

#### CA-Signed example

In this mode the TLS certificate is signed by a remote Certificate authority
The web server requires internet access, so you will need to forward port `80` and `443` on your router.

```
# Caddyfile
my.domain.com {
	redir / /live
	route / {
		reverse_proxy localhost:2020
		#reverse_proxy nvr:2020 # Docker
    }

    encode gzip

    header / {
		# Default security headers.
		# Enable HTTP Strict Transport Security (HSTS) to force clients to always
		# connect via HTTPS (do not use if only testing)
		Strict-Transport-Security "max-age=31536000;"
		# Enable cross-site filter (XSS) and tell browser to block detected attacks
		X-XSS-Protection "1; mode=block"
		# Prevent some browsers from MIME-sniffing a response away from the declared Content-Type
		X-Content-Type-Options "nosniff"
		# Disallow the site to be rendered within a frame (clickjacking protection)
		X-Frame-Options "DENY"
	}
}
```

Replace `my.domain.com` with your domain. Caddy will set up and manage the certificate automatically, if you don't own a domain you can use a free [Dynamic DNS service](https://www.comparitech.com/net-admin/dynamic-dns-providers/).

<br>

#### Self-Signed Example

In this mode Caddy will sign the certificate locally. You do not require internet access, but you will get an "Unknown Issuer" warning when you access the site.

```
# Caddyfile
:443 {
	tls internal {
		on_demand
	}

	redir / /live
	route / {
		reverse_proxy localhost:2020
		#reverse_proxy nvr:2020 # Docker
    }

    encode gzip

    header / {
		# Default security headers.
		# Enable HTTP Strict Transport Security (HSTS) to force clients to always
		# connect via HTTPS (do not use if only testing)
		Strict-Transport-Security "max-age=31536000;"
		# Enable cross-site filter (XSS) and tell browser to block detected attacks
		X-XSS-Protection "1; mode=block"
		# Prevent some browsers from MIME-sniffing a response away from the declared Content-Type
		X-Content-Type-Options "nosniff"
		# Disallow the site to be rendered within a frame (clickjacking protection)
		X-Frame-Options "DENY"
	}
}
```

<br>

## Before continuing

If the installation was successful, then you should now be able to access the debug page. `https://127.0.0.1/debug`


Please fix any errors before continuing to [Configuration](2_Configuration.md)
 
