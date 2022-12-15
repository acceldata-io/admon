// Acceldata Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// 	Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// admon is a daemon process to be ran by systemd
// It goes down only when It cannot read/access the 'admon.yml' config file

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/integrii/flaggy"
)

// Version will be set at the build time
var Version = "0.0.0"

var (
	// BuildID will be set at the build time
	BuildID          = "0"
	dockerAPIVersion = "1.41"
	configFileName   = "admon.yml"
	stateFile        = ".admon.state"
	lastErrorFile    = ".admon.lasterror"
	configDir        = "."
	containerNetwork = "all"
	runNow           = false
)

func init() {
	flaggy.SetName("Acceldata Admon")
	flaggy.SetDescription("Monitors the running containers and system resources in the local machine and sends alerts")

	// Shows help message when something went wrong
	flaggy.DefaultParser.ShowHelpOnUnexpected = true

	// Sets AD dev site for help message
	flaggy.DefaultParser.AdditionalHelpPrepend = "https://acceldata.io/"

	// set the version and parse all inputs into variables
	version := Version + "\n" + "Build ID: " + BuildID
	flaggy.SetVersion(version)

	//
	flaggy.String(&configDir, "c", "configdir", "Configuration Directory")
	flaggy.Bool(&runNow, "r", "run", "Runs the daemon")
	flaggy.String(&containerNetwork, "n", "network", "Container network name")

	//
	flaggy.Parse()

	//
	configDirEnv := strings.TrimSpace(os.Getenv("ADMON_CONFIGDIR"))
	if strings.TrimSpace(configDir) != "" {
		configDirInfo, err := os.Stat(configDir)
		if err != nil {
			fmt.Printf("ERROR: cannot find / access the directory %q because %s\n", configDir, err.Error())
			os.Exit(1)
		}

		if !configDirInfo.IsDir() {
			fmt.Printf("ERROR: the path %q is not a directory\n", configDir)
			os.Exit(1)
		}

	} else if configDirEnv != "" {
		configDir = configDirEnv
		configDirEnvInfo, err := os.Stat(configDirEnv)
		if err != nil {
			fmt.Printf("ERROR: cannot find / access the directory %q because %s\n", configDirEnv, err.Error())
			os.Exit(1)
		}

		if !configDirEnvInfo.IsDir() {
			fmt.Printf("ERROR: the path %q is not a directory\n", configDir)
			os.Exit(1)
		}
	} else {
		configDir = "."
	}

	// Initializes the config file
	initizeAdMon(dockerAPIVersion, containerNetwork, configDir, configFileName)
}

func main() {
	//
	if !runNow {
		fmt.Println("INFO: Pass the '-r' flag to run the daemon!")
		fmt.Println("INFO: Pass the '-h' flag to see help")
		os.Exit(0)
	}

	//
	configData, err := parseConfig(configDir, configFileName)
	if err != nil {
		fmt.Println("ERROR: ", err)
		os.Exit(1)
	}
	containerNetwork := configData.Network
	containersToCheck := configData.Containers
	slackTeamURL := configData.SlackTeamURL
	checkInterval := configData.CheckInterval
	snoozeTime := configData.SnoozeTime

	// System Metrics Checker - runs in a goroutine
	go func() {
		//
		fmt.Println("INFO: Initialised System Metric Checker ..")
		ticker := time.NewTicker(time.Duration(configData.SysConfig.CheckInterval) * time.Second)
		watcher := sysWatcher{
			cpuStatInterval: configData.SysConfig.CPUStatInterval,
			cpuThreshold:    configData.SysConfig.CPUThreshold,
			memThreshold:    configData.SysConfig.MemThreshold,
			diskThreshold:   configData.SysConfig.DiskThreshold,
			dirThreshold:    configData.SysConfig.DirThreshold,
		}
		// Initialise Alert Timers & Snooze Timers
		lastMailEpoch := time.Date(2020, time.April, 15, 0, 0, 0, 0, time.UTC)
		nextMailEpoch := time.Date(2020, time.April, 15, 0, 0, 0, 0, time.UTC)
		isFirstMail := true
		previousMessageLength := 0
		//
		for ; true; <-ticker.C {
			messages := watcher.watchSystemResources()
			//
			if len(messages) > 0 {

				fmt.Println("INFO: System Resources Reached Threshold ...")
				fmt.Println("INFO: ", messages)

				//
				currentTime := time.Unix(time.Now().Unix(), 0)

				// TODO: Needs a better logic -
				// Checks if it's sending the mail for the first time since the startup
				// If not checks if it reachs the beyond the snooze time
				// If not checks if the message length is changed compare to the previous message
				// Each message signifies some change in the state. But it doesn't mean changes are distinct
				// So this logic needs to be improved to check if the state is actually changed before sending an email
				if isFirstMail || (currentTime.After(nextMailEpoch) || previousMessageLength != len(messages)) {
					// send mail
					newMailConfig := mailConfig{
						SMTP:              configData.SMTP,
						MissingContainers: messages,
						SlackTeamURL:      slackTeamURL,
						APMServerIP:       configData.APMServerIP,
					}

					//
					fmt.Println("INFO: Trying to send the email ... ")
					if err := sendSysAlert(newMailConfig); err != nil {
						fmt.Println("ERROR:", err.Error())
					} else {
						fmt.Println("INFO: Email Sent!")
						isFirstMail = false
						lastMailEpoch = time.Unix(time.Now().Unix(), 0)
						nextMailEpoch = lastMailEpoch.Add(time.Duration(configData.SysConfig.SnoozeTime) * time.Second)
						previousMessageLength = len(messages)
					}
				} else {
					// Snooze
					waitTime := time.Unix(nextMailEpoch.Unix(), 0)
					fmt.Printf("INFO: Snoozing until - '%s'. Current time is: '%s'\n", waitTime.Format("2006-01-02T15:04:05.000Z"), time.Unix(time.Now().Unix(), 0).Format("2006-01-02T15:04:05.000Z"))
				}
			}
		}
	}()

	for {

		//
		fmt.Printf("INFO: Looking for containers in %q network ...\n", containerNetwork)
		missingContainers := []string{}

		stack, err := getRunningContainers(dockerAPIVersion, containerNetwork)
		if err == nil {
			missingContainers = sliceDiff(containersToCheck, stack)
		} else {
			fmt.Println("ERROR: Cannot get running containers. Because: ", err.Error())
			fmt.Println("INFO: Taking it as, all the containers are missing ...")
			missingContainers = containersToCheck
		}
		if len(missingContainers) > 0 {
			//
			fmt.Println("INFO: Missing Containers: ", missingContainers)

			//
			lastState, isFirstRun, err := getState(configDir, stateFile, missingContainers)
			if err == nil {
				//
				if !isFirstRun {
					// Compare States
					newState, toMail := compareStates(snoozeTime, lastState, getCurrentState(missingContainers))

					err := writeState(configDir, stateFile, newState)
					if err == nil {
						if toMail {
							// send mail
							newMailConfig := mailConfig{
								SMTP:              configData.SMTP,
								MissingContainers: missingContainers,
								SlackTeamURL:      slackTeamURL,
								APMServerIP:       configData.APMServerIP,
							}

							//
							fmt.Println("INFO: Trying to send the email ... ")
							if err := sendAlertMail(newMailConfig); err != nil {
								fmt.Println("ERROR:", err.Error())
							} else {
								fmt.Println("INFO: Email Sent!")
							}
						} else {
							fmt.Println("INFO: Snoozing ..")
						}
					} else {
						//
						errMsg := fmt.Sprintf("Cannot write to the state file at '%s'. Because: '%s'", configDir+"/tmp/"+stateFile, err.Error())
						fmt.Println("ERROR: ", errMsg)

						// Send mail
						newMailConfig := mailConfig{
							SMTP:         configData.SMTP,
							SlackTeamURL: slackTeamURL,
							APMServerIP:  configData.APMServerIP,
							ErrorMessage: errMsg,
						}

						//
						lastErrorTime, isNewError, err := getLastError(configDir, stateFile)
						if err != nil {
							fmt.Println("ERROR: Cannot check last error time. Because: ", err.Error())
						} else {
							if isNewError {
								fmt.Println("INFO: Trying to send the email ... ")
								if err := sendErrorMail(newMailConfig); err != nil {
									fmt.Println("ERROR: ", err.Error())
								} else {
									fmt.Println("INFO: Email Sent!")
								}
							} else {
								//
								if time.Now().Unix() >= time.Unix(lastErrorTime, 0).Add(time.Duration(snoozeTime)*time.Second).Unix() {
									//
									fmt.Println("INFO: Trying to send the email ... ")
									if err := sendErrorMail(newMailConfig); err != nil {
										fmt.Println("ERROR: ", err.Error())
									} else {
										fmt.Println("INFO: Email Sent!")
									}
								} else {
									fmt.Println("INFO: Snoozing!")
								}
							}
						}
					}
				} else {
					// This is the first run
					fmt.Println("INFO: This is first time I see containers missing!")
					err := writeState(configDir, stateFile, lastState)
					if err == nil {

						// Send Email
						newMailConfig := mailConfig{
							SMTP:              configData.SMTP,
							MissingContainers: missingContainers,
							SlackTeamURL:      slackTeamURL,
							APMServerIP:       configData.APMServerIP,
						}

						//
						fmt.Println("INFO: Trying to send the email ... ")
						if err := sendAlertMail(newMailConfig); err != nil {
							fmt.Println("ERROR: ", err.Error())
						} else {
							fmt.Println("INFO: Email Sent!")
						}
					} else {
						//
						errMsg := fmt.Sprintf("Cannot write to the state file at '%s'. Because: '%s'", configDir+"/tmp/"+stateFile, err.Error())
						fmt.Println("ERROR: ", errMsg)

						// Send mail
						newMailConfig := mailConfig{
							SMTP:         configData.SMTP,
							SlackTeamURL: slackTeamURL,
							APMServerIP:  configData.APMServerIP,
							ErrorMessage: errMsg,
						}

						//
						lastErrorTime, isNewError, err := getLastError(configDir, stateFile)
						if err != nil {
							fmt.Println("ERROR: Cannot check last error time. Because: ", err.Error())
						} else {
							if isNewError {
								fmt.Println("INFO: Trying to send the email ... ")
								if err := sendErrorMail(newMailConfig); err != nil {
									fmt.Println("ERROR: ", err.Error())
								} else {
									fmt.Println("INFO: Email Sent!")
								}
							} else {
								//
								if time.Now().Unix() >= time.Unix(lastErrorTime, 0).Add(time.Duration(snoozeTime)*time.Second).Unix() {
									//
									fmt.Println("INFO: Trying to send the email ... ")
									if err := sendErrorMail(newMailConfig); err != nil {
										fmt.Println("ERROR: ", err.Error())
									} else {
										fmt.Println("INFO: Email Sent!")
									}
								} else {
									fmt.Println("INFO: Snoozing!")
								}
							}
						}
					}
				}
			} else {
				//
				errMsg := fmt.Sprintf("Cannot get the state file at '%s'. Because: '%s'", configDir+"/tmp/"+stateFile, err.Error())
				fmt.Println("ERROR: ", errMsg)

				// Send mail
				newMailConfig := mailConfig{
					SMTP:         configData.SMTP,
					SlackTeamURL: slackTeamURL,
					APMServerIP:  configData.APMServerIP,
					ErrorMessage: errMsg,
				}

				//
				lastErrorTime, isNewError, err := getLastError(configDir, stateFile)
				if err != nil {
					fmt.Println("ERROR: Cannot check last error time. Because: ", err.Error())
				} else {
					if isNewError {
						fmt.Println("INFO: Trying to send the email ... ")
						if err := sendErrorMail(newMailConfig); err != nil {
							fmt.Println("ERROR: ", err.Error())
						} else {
							fmt.Println("INFO: Email Sent!")
						}
					} else {
						//
						if time.Now().Unix() >= time.Unix(lastErrorTime, 0).Add(time.Duration(snoozeTime)*time.Second).Unix() {
							//
							fmt.Println("INFO: Trying to send the email ... ")
							if err := sendErrorMail(newMailConfig); err != nil {
								fmt.Println("ERROR: ", err.Error())
							} else {
								fmt.Println("INFO: Email Sent!")
							}
						} else {
							fmt.Println("INFO: Snoozing!")
						}
					}
				}
			}
		} else {
			// Write empty state
			if err := writeState(configDir, stateFile, map[string]int64{}); err != nil {
				errMsg := fmt.Sprintf("Containers are running fine. But, cannot write to the state file at '%s'. Because: '%s'", configDir+"/tmp/"+stateFile, err.Error())
				fmt.Println("ERROR: ", errMsg)

				// Send mail
				newMailConfig := mailConfig{
					SMTP:         configData.SMTP,
					SlackTeamURL: slackTeamURL,
					APMServerIP:  configData.APMServerIP,
					ErrorMessage: errMsg,
				}

				//
				lastErrorTime, isNewError, err := getLastError(configDir, stateFile)
				if err != nil {
					fmt.Println("ERROR: Cannot check last error time. Because: ", err.Error())
				} else {
					if isNewError {
						fmt.Println("INFO: Trying to send the email ... ")
						if err := sendErrorMail(newMailConfig); err != nil {
							fmt.Println("ERROR: ", err.Error())
						} else {
							fmt.Println("INFO: Email Sent!")
						}
					} else {
						//
						if time.Now().Unix() >= time.Unix(lastErrorTime, 0).Add(time.Duration(snoozeTime)*time.Second).Unix() {
							//
							fmt.Println("INFO: Trying to send the email ... ")
							if err := sendErrorMail(newMailConfig); err != nil {
								fmt.Println("ERROR: ", err.Error())
							} else {
								fmt.Println("INFO: Email Sent!")
							}
						} else {
							fmt.Println("INFO: Snoozing!")
						}
					}
				}
			} else {
				fmt.Println("INFO: Everything Looks Good!")
			}
		}

		// Check interval
		time.Sleep(time.Duration(checkInterval) * time.Second)
	}
}
