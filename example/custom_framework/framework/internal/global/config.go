// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package global

import "github.com/z5labs/bedrock/pkg/config"

var ConfigSources []config.Source

func RegisterConfigSource(src config.Source) {
	ConfigSources = append(ConfigSources, src)
}
