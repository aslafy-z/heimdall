// Copyright 2022 Dimitrij Drus <dadrus@gmx.de>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigurationFromStructWithDefaultsOnly(t *testing.T) {
	t.Parallel()

	// WHEN
	config, err := NewConfiguration("HEIMDALLCFG_", "")

	// THEN
	require.NoError(t, err)
	require.Equal(t, defaultConfig, *config)
}

func TestNewConfigurationWithConfigFile(t *testing.T) {
	t.Parallel()

	// WHEN
	config, err := NewConfiguration("HEIMDALLCFG_", "./test_data/test_config.yaml")

	// THEN
	require.NoError(t, err)
	assert.NotEqual(t, defaultConfig, config)
}
