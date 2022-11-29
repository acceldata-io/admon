# Acceldata Admon

Monitors the locally running docker containers and the basic system resources.

---

## Compile the `admon` binary for Linux

```shell
env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -a -installsuffix cgo -ldflags='-w -s' -o admon
```

---

## Usage

1. Run the binary once to generate the config file

    ```shell
    ./admon
    ```

    Expected output:

    ```shell
    INFO: Config file initiated successfully!
    INFO: Edit the config file at './admon.yml'
    ```

2. Edit the config file `./admon.yml` and customise the options

   * Properties in the configuration file that may need to be updated

        ```text
        PULSE_SERVER_HOSTNAME/IP
        
        containers - Validate if the list of containers are what we want to monitor in the Pulse server machine.

        SMTP_SERVER_USERNAME

        SMTP_SERVER_PASSWORD

        SMTP_SERVER_HOST/IP

        SMTP_SERVER_PORT

        SENDER_EMAIL_ADDRESS

        SENDER_NAME

        RECEIVER_EMAIL_ADDRESS_1

        RECEIVER_EMAIL_ADDRESS_2

        CONTAINER_MONITORING_CHECK_INTERVAL_IN_SECONDS  - default value: 60

        CONTAINER_MONITORING_SNOOZE_TIME_IN_SECONDS  - default value: 360

        DISK_THRESHOLD_PERCENTAGE  - default value: 0 (disabled)

        DISK_MONITORING_CHECK_INTERVAL_IN_SECONDS   - default value: 60

        DISK_MONITORING_SNOOZE_TIME_IN_SECONDS   - default value: 360

        ```

   * Example Scenarios for email alerts:
     * Check if any containers are not running every 60 seconds interval, in-case any container is down send an alert email and do not resend (snooze) the alert for the next 6 minutes unless any new containers failed.
       * set CONTAINER_MONITORING_CHECK_INTERVAL_IN_SECONDS to 60 .
       * set CONTAINER_MONITORING_SNOOZE_TIME_IN_SECONDS to 360 .
       * Example YAML configuration block:

        ```yaml
        CheckInterval: 60
        SnoozeTime: 360
        ```

     * Check if the disk usage has gone beyond or equal to 80% for the mount point / every 60 seconds interval and send an email alert in-case of reaching the threshold percentage and do not resend (snooze) the alert for the next 6 minutes.
       * set DISK_THRESHOLD_PERCENTAGE for the mount-point / to 80 .
       * set DISK_MONITORING_CHECK_INTERVAL_IN_SECONDS to 60 .
       * set DISK_MONITORING_SNOOZE_TIME_IN_SECONDS to 360 .
       * Example YAML configuration block:

        ```yaml
        sysConfig:
        diskThreshold:
            /: 80
        checkInterval: 60
        SnoozeTime: 360
        ```

   * You can see all mount-points available in a system using the following command

    ```shell
    $ df -h

    Filesystem      Size  Used Avail Use% Mounted on
    dev             7.8G     0  7.8G   0% /dev
    run             7.8G  1.7M  7.8G   1% /run
    /dev/nvme0n1p1  246G  132G  104G  50% /
    tmpfs           7.8G  232M  7.6G   3% /dev/shm
    tmpfs           7.8G   60M  7.7G   1% /tmp
    /dev/nvme0n1p3  500M  280K  499M   1% /boot/efi
    tmpfs           1.6G  108K  1.6G   1% /run/user/1000
    ```

3. Run the admon binary with the flag `-r`

   ```shell
   ./admon -r
   ```

   Expected output:

   ```shell
   INFO: Looking for containers in "all" network ...
   INFO: Initialised System Metric Checker ..
   INFO: Everything Looks Good!
   ```

4. If any of the container goes down `admon` will print the below in the stdout and tries to send an email using the `SMTP` configuraion from the config file

    ```shell
    INFO: Looking for containers in "all" network ...
    ERROR: Cannot get running containers. Because:  No existing containers found
    INFO: Taking it as, all the containers are missing ...
    INFO: Missing Containers:  [webserver_1]
    INFO: Trying to send the email ...
    ```

---

## Creating a `systemd` service for `admon`

* Create a new file at the path `/etc/systemd/system/admon.service` with the below contents:
  * Replace `</PATH/TO>` and `<CONFIG_DIR>` appropriate values

    ```shell
    [Unit]
    Description=AccelData AdMon Daemon
    Wants=syslog.target network.target
    After=syslog.target network.target
    [Service]
    Type=simple
    PIDFile=/run/admon.pid
    ExecStartPre=/usr/bin/test -f /usr/bin/docker
    ExecStart=</PATH/TO>/admon -r -c <CONFIG_DIR>
    ExecStop=/bin/kill -9 $MAINPID
    TimeoutStopSec=10
    Restart=always
    RestartSec=10
    [Install]
    ```

* Run below commands to start & enable admon as a systemd service:

    ```shell
    sudo systemctl daemon-reload
    sudo systemctl start admon
    sudo systemctl status admon
    ```

* Enable the `admon` systemd service so that it will start automatically whenever the system reboots

    ```shell
    sudo systemctl enable admon
    ```

---

## Troubleshooting

* If the user running the `admon` binary/service doesn't have the required system permissions you'll see the errors like

    ```shell
    ERROR: Cannot fetch disk usage for the path '/run/user/1000/doc'. Because: operation not permitted
    ERROR: Cannot fetch disk usage for the path '/var/lib/docker/overlay2/e0b75929a687e684ad36cd16b018a78b5843deb47257bef96648958fdaac4112/merged'. Because: permission denied
    ERROR: Cannot fetch disk usage for the path '/run/docker/netns/edaa4cf324e6'. Because: permission denied
    ```

* If no docker containers were running in your machine you'll see the below error

    ```shell
    ERROR:  No existing containers found
    ```

* If the `SMTP` server configured is not working you'll see the below error

    ```shell
    ERROR: Failed while dialing for alert mail ..
    ERROR: dial tcp: lookup smtp-us-email.server.net on 127.0.0.53:53: no such host
    ```

---
