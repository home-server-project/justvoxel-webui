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

## Local network access

JustVoxel WebUI is designed for simple administration from a trusted local home network. By default, Web management uses plain HTTP on port `8099` so the appliance can be opened directly from its local IP address without requiring users to install a private certificate or bypass browser certificate warnings.

Because default local WebUI traffic is not protected by TLS, administrator credentials and sessions should only be used on a network you trust. This is an intentional local-appliance design choice, not a missing configuration step.

Do not forward the JustVoxel WebUI management port directly to the public Internet.

Administrators who need remote or public access can add a properly secured HTTPS solution separately, such as a trusted reverse proxy, VPN, or private-network access service. Those configurations are outside the default JustVoxel local-access setup.

## License

Apache License 2.0.
