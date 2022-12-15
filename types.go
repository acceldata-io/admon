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

package main

type adMonConfig struct {
	Network       string     `yaml:"network"`
	APMServerIP   string     `yaml:"apmServerIP"`
	Containers    []string   `yaml:"containers"`
	SMTP          smtpConfig `yaml:"smtp"`
	SlackTeamURL  string     `yaml:"slackTeamURL"`
	CheckInterval int        `yaml:"CheckInterval"`
	SnoozeTime    int        `yaml:"SnoozeTime"`
	SysConfig     sysConfig  `yaml:"sysConfig"`
}

type sysConfig struct {
	CPUStatInterval int                `yaml:"cpuStatInterval,omitempty"`
	CPUThreshold    float64            `yaml:"cpuThreshold,omitempty"`
	MemThreshold    float64            `yaml:"memThreshold,omitempty"`
	DiskThreshold   map[string]float64 `yaml:"diskThreshold"`
	DirThreshold    map[string]int64   `yaml:"dirThreshold,omitempty"`
	CheckInterval   int                `yaml:"checkInterval"`
	SnoozeTime      int                `yaml:"SnoozeTime"`
}

type smtpConfig struct {
	Username        string   `yaml:"username"`
	Password        string   `yaml:"password"`
	Server          string   `yaml:"server"`
	Port            int      `yaml:"port"`
	SenderAddr      string   `yaml:"sender"`
	SenderName      string   `yaml:"senderName"`
	ReceiverAddrs   []string `yaml:"receivers"`
	EmailSubject    string   `yaml:"emailSubject"`
	SysAlertSubject string   `yaml:"sysAlertSubject"`
	AuthEnabled     bool     `yaml:"authEnabled"`
}

type mailConfig struct {
	SMTP              smtpConfig
	MissingContainers []string
	SlackTeamURL      string
	APMServerIP       string
	ErrorMessage      string
}
