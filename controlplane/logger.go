// Package controlplane Copyright 2020 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
package controlplane

import (
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"log"
)

// Logger An controlplane of a logger that implements `pkg/log/Logger`.  Logs to
// stdout.  If Debug == false then Debugf() and Infof() won't output
// anything.
type Logger struct {
	Debug bool
	logr.Logger
}

// Debugf Log to stdout only if Debug is true.
func (logger Logger) Debugf(format string, args ...interface{}) {
	if logger.Debug {
		logger.Logger.Info("DEBUG: "+format+"\n", args...)
	}
}

// Infof Log to stdout only if Debug is true.
func (logger Logger) Infof(format string, args ...interface{}) {
	if logger.Debug {
		logger.Logger.Info("INFO: "+format+"\n", args...)
	}
}

// Warnf Log to stdout always.
func (logger Logger) Warnf(format string, args ...interface{}) {
	logger.Logger.Info("WARNING: "+format+"\n", args...)
}

// Errorf Log to stdout always.
func (logger Logger) Errorf(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
	str := fmt.Sprintf("%v", args)
	logger.Logger.Error(errors.New(str), "ERROR: "+format+"\n")
}
