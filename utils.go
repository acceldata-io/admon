package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/gomail.v2"
	"gopkg.in/yaml.v2"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

func getRunningContainers(dockerAPIVersion, containerNetwork string) ([]string, error) {
	stack := []string{}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.WithVersion(dockerAPIVersion))
	if err != nil {
		fmt.Println("ERROR: Failed to aquire docker API client")
		return stack, err
	}

	defer cli.Close()

	//
	clFilters := filters.NewArgs()
	if containerNetwork != "all" {
		clFilters.Add("network", containerNetwork)
	}
	clFilters.Add("status", "running")
	containerOpts := types.ContainerListOptions{
		Filters: clFilters,
	}

	// Get a list of locally available containers in any states and attached to any networks
	localContainers, err := cli.ContainerList(ctx, containerOpts)
	if err != nil {
		fmt.Println("ERROR: Failed to get local containers list from API")
		return stack, err
	}

	// Map each container to it's network stack
	if len(localContainers) != 0 {
		for _, container := range localContainers {
			stack = append(stack, strings.TrimPrefix(container.Names[0], "/"))
		}
		return stack, nil
	}
	err = errors.New("No existing containers found")
	return stack, err
}

func sliceDiff(a, b []string) []string {
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}

func sendAlertMail(mail mailConfig) error {
	// The email body for recipients with non-HTML email clients.
	textBody := "This following containers are not running.\n"
	textBody = textBody + strings.Join(mail.MissingContainers, ",")

	//
	// Create a new message.
	m := gomail.NewMessage()

	//
	alertTemplate := template.New("alert.html")

	alertTemplate, err := alertTemplate.Parse(alertMailTemplate)
	if err != nil {
		fmt.Println("ERROR: Cannot parse the alert email template")
		return err
	}

	//
	var alertMail bytes.Buffer
	if err := alertTemplate.Execute(&alertMail, mail); err != nil {
		fmt.Println("ERROR: Cannot execute alert email template")
		return err
	}

	mailBody := alertMail.String()

	// set the email body to html
	m.SetBody("text/html", mailBody)

	// Set the alternate email body to plain text.
	// m.AddAlternative("text/plain", textBody)

	// Construct the message headers, including a Configuration Set and a Tag.
	m.SetHeaders(map[string][]string{
		"From":    {m.FormatAddress(mail.SMTP.SenderAddr, mail.SMTP.SenderName)},
		"Subject": {mail.SMTP.EmailSubject},
	})

	m.SetHeader("To", mail.SMTP.ReceiverAddrs...)

	// Send the email.
	var d *gomail.Dialer
	if mail.SMTP.AuthEnabled {
		d = gomail.NewPlainDialer(mail.SMTP.Server, mail.SMTP.Port, mail.SMTP.Username, mail.SMTP.Password)
	} else {
		d = &gomail.Dialer{Host: mail.SMTP.Server, Port: mail.SMTP.Port}
	}

	// Display an error message if something goes wrong; otherwise,
	// display a message confirming that the message was sent.
	if err := d.DialAndSend(m); err != nil {
		fmt.Println("ERROR: Failed while dialing for alert mail ..")
		return err
	}
	return nil
}

func readFile(fileLocation string) ([]byte, error) {
	cfg, err := os.Open(fileLocation)
	if err != nil {
		fmt.Println("ERROR: Cannot open the file: ", fileLocation)
		return []byte(""), err
	}

	defer cfg.Close()

	byteValue, err := ioutil.ReadAll(cfg)
	if err != nil {
		fmt.Println("ERROR: Cannot read the file: ", fileLocation)
		return []byte(""), err
	}
	return byteValue, nil
}

func parseConfig(configDir, configFileName string) (adMonConfig, error) {
	//
	configFile := configDir + "/" + configFileName

	configData := adMonConfig{}
	configFileData, err := readFile(configFile)
	if err != nil {
		return configData, err
	}
	err = yaml.Unmarshal(configFileData, &configData)
	if err != nil {
		fmt.Println("ERROR: Cannot unmarshal '" + configFile + "' file")
		return configData, err
	}
	return configData, nil
}

func writeConfig(filePath, fileName string, fileData []byte) error {
	//
	file := filePath + "/" + fileName
	// Try to write the file
	if err := ioutil.WriteFile(file, fileData, 0o744); err != nil {
		fmt.Println("ERROR: Failed to write the file: ", file)
		return err
	}

	return nil
}

func initizeAdMon(dockerAPIVersion, containerNetwork, configDir, fileName string) {
	//
	configFile := configDir + "/" + fileName
	//
	if _, err := os.Stat(configFile); os.IsNotExist(err) {

		//
		runningContainers, err := getRunningContainers(dockerAPIVersion, containerNetwork)
		if err != nil {
			fmt.Println("ERROR: ", err)
			os.Exit(1)
		}

		defaultConfig := getDefaultConfig(runningContainers, containerNetwork)
		configData, err := yaml.Marshal(&defaultConfig)
		if err != nil {
			fmt.Println("ERROR: Cannot marshel config data. Because: ", err.Error())
			os.Exit(1)
		}

		//
		if err := writeConfig(configDir+"/", fileName, configData); err != nil {
			fmt.Println("ERROR: Cannot write config file. Because: ", err.Error())
			os.Exit(1)
		}

		//
		fmt.Println("INFO: Config file initiated successfully!")
		fmt.Printf("INFO: Edit the config file at '%s'\n", configFile)
		os.Exit(0)
	} else if err != nil {
		fmt.Println("ERROR: Cannot access configuration directory. Because: ", err.Error())
		os.Exit(1)
	}
}

func getDefaultConfig(runningContainers []string, containerNetwork string) adMonConfig {
	//
	defaultSMTPConfig := smtpConfig{
		Username:        "testUser",
		Password:        "d0MewxvQ6iiOrDFr/E6LoA==",
		Server:          "smtp-us-email.server.net",
		Port:            587,
		SenderName:      "Admon",
		SenderAddr:      "admon@tntcorp.com",
		ReceiverAddrs:   []string{"dev1@tntcorp.com", "dev2@tntcorp.com"},
		EmailSubject:    "[ALERT] Containers Not Running | Admon",
		SysAlertSubject: "[ALERT] Server Resources Reached Threshold | Admon",
		AuthEnabled:     true,
	}
	//
	defaultSysConfig := sysConfig{
		CheckInterval: 60,
		SnoozeTime:    360,
		DiskThreshold: map[string]float64{
			"/":     0,
			"/root": 0,
		},
	}
	//
	defaultConfig := adMonConfig{
		Network:       containerNetwork,
		APMServerIP:   getOutboundIP().String(),
		Containers:    runningContainers,
		SMTP:          defaultSMTPConfig,
		SlackTeamURL:  "",
		CheckInterval: 60,
		SnoozeTime:    360,
		SysConfig:     defaultSysConfig,
	}

	return defaultConfig
}

func getState(configDir, fileName string, containers []string) (map[string]int64, bool, error) {
	//
	stateFilePath := configDir + "/" + fileName
	containerMap := make(map[string]int64)
	isNew := true

	//
	_, err := os.Stat(stateFilePath)
	if os.IsNotExist(err) {

		//
		for _, containerName := range containers {
			containerMap[containerName] = time.Now().Unix()
		}

		//
		stateData, err := json.Marshal(containerMap)
		if err != nil {
			fmt.Println("ERROR: Cannot marshel container map")
			return containerMap, isNew, err
		}

		//
		err = ioutil.WriteFile(stateFilePath, stateData, 0o644)
		if err != nil {
			fmt.Println("ERROR: Cannot write the state file")
			return containerMap, isNew, err
		}
		return containerMap, isNew, nil
	} else if err != nil {
		fmt.Printf("ERROR: Cannot check the state file at '%s'\n", stateFilePath)
		return containerMap, false, err
	}
	// State file exists, Just read and return
	isNew = false
	stateData, err := ioutil.ReadFile(stateFilePath)
	if err == nil {
		if err := json.Unmarshal(stateData, &containerMap); err != nil {
			fmt.Println("ERROR: Cannot unmarshal existing state file")
			return containerMap, isNew, err
		}
		return containerMap, isNew, nil
	}
	return containerMap, isNew, err
}

func getCurrentState(containers []string) map[string]int64 {
	//
	containerMap := make(map[string]int64)
	//
	for _, containerName := range containers {
		containerMap[containerName] = time.Now().Unix()
	}
	return containerMap
}

func writeState(configDir, fileName string, containerMap map[string]int64) error {
	//
	stateFilePath := configDir + "/" + fileName
	//
	stateData, err := json.Marshal(containerMap)
	if err != nil {
		fmt.Println("ERROR: Cannot marshel container map")
		return err
	}

	//
	err = ioutil.WriteFile(stateFilePath, stateData, 0o644)
	if err != nil {
		fmt.Println("ERROR: Cannot write the state file")
		return err
	}

	return nil
}

func compareStates(snoozeTime int, lastState, currentState map[string]int64) (map[string]int64, bool) {
	//
	c1diffState := make(map[string]int64)
	l2diffState := make(map[string]int64)
	c2diffState := make(map[string]int64)
	updatedState := make(map[string]int64)
	toMail := true

	//
	for container, currentTime := range currentState {
		//
		if lastTime, ok := lastState[container]; !ok {
			//
			c1diffState[container] = currentTime
		} else {
			l2diffState[container] = lastTime
			c2diffState[container] = currentTime
		}
	}

	// Check if we get any difference from last state of missing containers
	if len(c1diffState) > 0 {
		// New containers are missing compared to last state
		// Merge the newly missing containers with the existing list of missing containers
		// Send the mail
		updatedState = mergeMaps(c1diffState, c2diffState)
	} else {
		// No new missing containers
		// Now, decide whether to update the state and/or send the mail
		var (
			cT1 int64
			lT1 int64
		)
		// Get the current time
		for _, currentTime := range c2diffState {
			//
			cT1 = currentTime
			break
		}

		// Get the last time
		for _, lastTime := range l2diffState {
			//
			lT1 = lastTime
			break
		}

		// Check if the current time is exceeding the last time + snooze time
		if cT1 >= (time.Unix(lT1, 0).Add(time.Duration(snoozeTime) * time.Second).Unix()) {
			// Exceeded the snooze time
			// Update the state with current time and send the mail
			updatedState = c2diffState
		} else {
			// Within the snooze time
			// Pass the last state as updated state and do not send the mail
			updatedState = l2diffState
			toMail = false
		}
	}
	return updatedState, toMail
}

func mergeMaps(mapOne, mapTwo map[string]int64) map[string]int64 {
	//
	for k, v := range mapTwo {
		mapOne[k] = v
	}

	return mapOne
}

func sendErrorMail(mail mailConfig) error {
	//
	//The email body for recipients with non-HTML email clients.
	textBody := "Somthing went wrong with the server.\n"
	textBody = textBody + "\n" + mail.ErrorMessage

	//
	// Create a new message.
	m := gomail.NewMessage()

	//
	errorTemplate := template.New("error.html")

	errorTemplate, err := errorTemplate.Parse(errorMailTemplate)
	if err != nil {
		fmt.Println("ERROR: Cannot parse the error email template")
		return err
	}

	//
	var errorMail bytes.Buffer
	if err := errorTemplate.Execute(&errorMail, mail); err != nil {
		fmt.Println("ERROR: Cannot execute error email template")
		return err
	}

	mailBody := errorMail.String()

	// set the email body to html
	m.SetBody("text/html", mailBody)

	// Set the alternate email body to plain text.
	// m.AddAlternative("text/plain", textBody)

	// Construct the message headers, including a Configuration Set and a Tag.
	m.SetHeaders(map[string][]string{
		"From":    {m.FormatAddress(mail.SMTP.SenderAddr, mail.SMTP.SenderName)},
		"Subject": {mail.SMTP.EmailSubject},
	})

	m.SetHeader("To", mail.SMTP.ReceiverAddrs...)

	// Send the email.
	var d *gomail.Dialer
	if mail.SMTP.AuthEnabled {
		d = gomail.NewPlainDialer(mail.SMTP.Server, mail.SMTP.Port, mail.SMTP.Username, mail.SMTP.Password)
	} else {
		d = &gomail.Dialer{Host: mail.SMTP.Server, Port: mail.SMTP.Port}
	}

	// Display an error message if something goes wrong; otherwise,
	// display a message confirming that the message was sent.
	if err := d.DialAndSend(m); err != nil {
		fmt.Println("ERROR: Failed while dialing for error mail ..")
		return err
	}
	return nil
}

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println("ERROR: Cannot get the outbound IP. Because: ", err.Error())
		fmt.Println("ERROR: Please set the outbound IP manually in the 'admon.yml' file!")
		return net.IPv4(0, 0, 0, 0)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func getLastError(configDir, fileName string) (int64, bool, error) {
	//
	stateFilePath := configDir + "/tmp/" + fileName
	timeStamp := time.Now().Unix()
	isNew := true

	//
	_, err := os.Stat(stateFilePath)
	if os.IsNotExist(err) {

		//
		stateData, err := json.Marshal(timeStamp)
		if err != nil {
			fmt.Println("ERROR: Cannot marshel container map")
			return timeStamp, isNew, err
		}

		//
		err = ioutil.WriteFile(stateFilePath, stateData, 0o644)
		if err != nil {
			fmt.Println("ERROR: Cannot write the last error state file")
			return timeStamp, isNew, err
		}
		return timeStamp, isNew, nil
	} else if err != nil {
		fmt.Printf("ERROR: Cannot check the last error state file at '%s'\n", stateFilePath)
		return timeStamp, false, err
	}
	// State file exists, Just read and return
	isNew = false
	stateData, err := ioutil.ReadFile(stateFilePath)
	if err == nil {
		if err := json.Unmarshal(stateData, &timeStamp); err != nil {
			fmt.Println("ERROR: Cannot unmarshal existing state file")
			return timeStamp, isNew, err
		}
		return timeStamp, isNew, nil
	}
	return timeStamp, isNew, err
}

func sendSysAlert(mail mailConfig) error {
	// The email body for recipients with non-HTML email clients.
	textBody := "These following system resources reached threshold.\n"
	textBody = textBody + strings.Join(mail.MissingContainers, ",")

	//
	// Create a new message.
	m := gomail.NewMessage()

	//
	alertTemplate := template.New("alert.html")

	alertTemplate, err := alertTemplate.Parse(sysAlertMailTemplate)
	if err != nil {
		fmt.Println("ERROR: Cannot parse the alert email template")
		return err
	}

	//
	var alertMail bytes.Buffer
	if err := alertTemplate.Execute(&alertMail, mail); err != nil {
		fmt.Println("ERROR: Cannot execute alert email template")
		return err
	}

	mailBody := alertMail.String()

	// set the email body to html
	m.SetBody("text/html", mailBody)

	// Set the alternate email body to plain text.
	// m.AddAlternative("text/plain", textBody)

	// Construct the message headers, including a Configuration Set and a Tag.
	m.SetHeaders(map[string][]string{
		"From":    {m.FormatAddress(mail.SMTP.SenderAddr, mail.SMTP.SenderName)},
		"Subject": {mail.SMTP.SysAlertSubject},
	})

	m.SetHeader("To", mail.SMTP.ReceiverAddrs...)

	// Send the email.
	var d *gomail.Dialer
	if mail.SMTP.AuthEnabled {
		d = gomail.NewPlainDialer(mail.SMTP.Server, mail.SMTP.Port, mail.SMTP.Username, mail.SMTP.Password)
	} else {
		d = &gomail.Dialer{Host: mail.SMTP.Server, Port: mail.SMTP.Port}
	}

	// Display an error message if something goes wrong; otherwise,
	// display a message confirming that the message was sent.
	if err := d.DialAndSend(m); err != nil {
		fmt.Println("ERROR: Failed while dialing for alert mail ..")
		return err
	}
	return nil
}
