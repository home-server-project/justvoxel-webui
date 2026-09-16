# JustVoxel WebUI

Web management interface for the JustVoxel Minecraft Server Appliance.

This repository owns the unprivileged browser-facing Web application. Privileged appliance operations, authentication state, system integration, and the management API remain in the main `home-server-project/justvoxel` repository.

The WebUI is built as a versioned release artifact and baked into JustVoxel bootc images. End users update it through the normal JustVoxel system update path; there is no separate WebUI update lifecycle.

## Development status

Early development. Do not use as a standalone administration service.

## Branch model

- `testing` - active development and prerelease validation
- `main` - approved stable source

## Runtime model

The WebUI runs as an unprivileged native systemd service inside JustVoxel and communicates with the local privileged JustVoxel Management API through a Unix socket. It does not receive a Podman socket, unrestricted systemd control, arbitrary shell execution, or unrestricted host filesystem access.

## Authentication

The default administrator identity is `voxel` and the default authentication mode is **System account**. In this mode, the privileged JustVoxel Management Agent authenticates the real local `voxel` account through the AlmaLinux/RHEL PAM stack. The unprivileged WebUI does not read `/etc/shadow`, store a synchronized copy of the Linux password, or call PAM directly.

A successful system-account login becomes a normal JustVoxel WebUI session. The Linux password is not resent for ordinary management requests.

Administrators can optionally select **Separate WebUI password** mode. That provider uses its own WebUI-local credential and does not modify the Linux/console/SSH password.

## Local network access

JustVoxel WebUI is designed for simple administration from a trusted local home network. By default, Web management uses plain HTTP on port `8099` so the appliance can be opened directly from its local IP address without requiring users to install a private certificate or bypass browser certificate warnings.

Because default local WebUI traffic is not protected by TLS, administrator credentials and sessions should only be used on a network you trust. In System account mode the WebUI password is also the system administrator password. This is an intentional local-appliance design choice, not a missing configuration step.

Do not forward the JustVoxel WebUI management port directly to the public Internet.

Administrators who need remote or public access can add a properly secured HTTPS solution separately, such as a trusted reverse proxy, VPN, or private-network access service. Those configurations are outside the default JustVoxel local-access setup.

## License

Apache License 2.0.
