// Copyright 2022, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mlc

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

type logEmpty struct{}

// Debug logs a debug-level message that is generally hidden from end-users.
func (log *logEmpty) Debug(msg string, args *pulumi.LogArgs) error {
	return nil
}

// Logs an informational message that is generally printed to stdout during resource
func (log *logEmpty) Info(msg string, args *pulumi.LogArgs) error {
	return nil
}

// Logs a warning to indicate that something went wrong, but not catastrophically so.
func (log *logEmpty) Warn(msg string, args *pulumi.LogArgs) error {
	return nil
}

// Logs a fatal condition. Consider returning a non-nil error object
// after calling Error to stop the Pulumi program.
func (log *logEmpty) Error(msg string, args *pulumi.LogArgs) error {
	return nil
}
