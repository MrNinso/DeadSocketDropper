# DeadSocketDropper

A Go application designed to monitor, track, and automatically terminate hanging or long-lived TCP connections on a Linux host using the `ss --kill` utility. Runs efficiently as a Docker container.

## Features

*   **Real-time Monitoring:** Tracks connections on a specific TCP source port every 30 minutes (configurable).
*   **Inode Identification:** Uses the kernel's unique identifier (inode) to track connections precisely.
*   **Automatic Termination:** Kills connections that remain active for more than 2 hours (configurable).
*   **List Cleanup:** Removes connections from the tracking list that haven't been seen in over 1 hour (configurable).
*   **Dockerized:** Packaged for easy deployment using `docker-compose` with necessary host network privileges.

## Prerequisites

*   **Linux Host OS:** The `ss` utility and `ss --kill` functionality are Linux-specific.
*   **Docker and Docker Compose:** To build and run the service easily.

## How to Use

### 1. Clone the Repository

```bash
git clone github.com
cd DeadSocketDropper
```

### 2. Configuration (Optional)

Default parameters are set in the docker-compose.yml file, but you can adjust them:
```yaml
# docker-compose.yml
services:
  connection-monitor:
    # ... (other configurations) ...
    command: 
      [
        "-port=${PORT}",          # Source port to monitor
        "-check-interval=${CHECK_INTEVAL}",   # Check interval in minutes
        "-max-active=${MAX_ACTIVE}",      # Maximum allowed active duration in minutes (2 hours)
        "-max-inactive=${MAX_INACTIVE}"      # Time unused before being removed from list (1 hour)
      ]

```

### 3. Run with Docker Compose

The service requires host network access and administrative privileges (NET_ADMIN, SYS_ADMIN) to function correctly.

```bash
# Build the image and start the container in the background (-d)
sudo docker compose up --build -d
```

### Contributing
Feel free to open issues or pull requests in the repository.


