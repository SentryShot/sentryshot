# Installation

- [Automatic installation](#automatic-installation)
- [Manual installation](#manual-installation)
- [Webserver](#web-server)
- [Before continuing](#before-continuing)

<br>

## Automatic installation

    wget https://gitlab.com/osnvr/os-nvr_assets/-/raw/master/installer.sh && sudo ./install.sh

<br>

## Manual Installation

Install dependencies

- [Golang](https://golang.org/doc/install) 1.15+
- [ffmpeg](https://ffmpeg.org/download.html) 4.3+
- `git sed which`

Create an unprivileged user named `_nvr`

    sudo useradd -m -s /sbin/nologin _nvr

Go to `_nvr` home and clone repository.

    cd /home/_nvr/
    sudo -u _nvr git clone --branch master https://gitlab.com/osnvr/os-nvr.git
    cd ./os-nvr

Copy sample configs.

    sudo -u _nvr cp ./configs/.samples/* ./configs/

Check if the values in `env.json`are correct.

    sudo -u _nvr nano ./configs/env.json

Download Golang dependencies.
	
	sudo -u _nvr go mod download

Run service creation script

    sudo ./utils/services/systemd.sh --name=nvr --cmd="/usr/bin/go run /home/_nvr/os-nvr/start/start.go"

<br>

## Web server

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
Default login: `admin:pass`

If the installation was successful, then you should now be able to access the debug page `https://my.domain.com/debug` 


Please fix any errors before continuing to [Configuration](2_Configuration.md)
 