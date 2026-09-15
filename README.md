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

## License

Apache License 2.0.
