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

package httpendpoint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dadrus/heimdall/internal/app"
	"github.com/dadrus/heimdall/internal/cache/memory"
	"github.com/dadrus/heimdall/internal/config"
	"github.com/dadrus/heimdall/internal/heimdall"
	config2 "github.com/dadrus/heimdall/internal/rules/config"
	"github.com/dadrus/heimdall/internal/rules/rule/mocks"
	"github.com/dadrus/heimdall/internal/validation"
	"github.com/dadrus/heimdall/internal/x"
	"github.com/dadrus/heimdall/internal/x/testsupport"
	mock2 "github.com/dadrus/heimdall/internal/x/testsupport/mock"
)

func TestNewProvider(t *testing.T) {
	t.Parallel()

	for uc, tc := range map[string]struct {
		enforceTLS bool
		conf       []byte
		assert     func(t *testing.T, err error, prov *Provider)
	}{
		"with unknown field": {
			conf: []byte(`foo: bar`),
			assert: func(t *testing.T, err error, _ *Provider) {
				t.Helper()

				require.Error(t, err)
				require.ErrorIs(t, err, heimdall.ErrConfiguration)
				assert.Contains(t, err.Error(), "failed decoding")
			},
		},
		"without endpoints": {
			conf: []byte(`watch_interval: 5s`),
			assert: func(t *testing.T, err error, _ *Provider) {
				t.Helper()

				require.Error(t, err)
				require.ErrorIs(t, err, heimdall.ErrConfiguration)
				assert.Contains(t, err.Error(), "'endpoints' is a required field")
			},
		},
		"with watch interval and unsupported endpoint property configured": {
			conf: []byte(`
watch_interval: 5s
endpoints:
- foo: bar
`),
			assert: func(t *testing.T, err error, _ *Provider) {
				t.Helper()

				require.Error(t, err)
				require.ErrorIs(t, err, heimdall.ErrConfiguration)
				assert.Contains(t, err.Error(), "failed decoding")
			},
		},
		"with one endpoint without url": {
			conf: []byte(`
endpoints:
- method: POST
`),
			assert: func(t *testing.T, err error, _ *Provider) {
				t.Helper()

				require.Error(t, err)
				require.ErrorIs(t, err, heimdall.ErrConfiguration)
				assert.Contains(t, err.Error(), "'endpoints'[0].'url' is a required field")
			},
		},
		"with enforced TLS, only one endpoint and its url configured to use TLS": {
			enforceTLS: true,
			conf: []byte(`
endpoints:
- url: https://foo.bar
`),
			assert: func(t *testing.T, err error, prov *Provider) {
				t.Helper()

				require.NoError(t, err)
				require.NotNil(t, prov)
				assert.NotNil(t, prov.s)
				assert.NotNil(t, prov.p)
				assert.NotNil(t, prov.cancel)

				time.Sleep(250 * time.Millisecond)

				jobs := prov.s.Jobs()
				assert.Len(t, jobs, 1)

				lr, err := jobs[0].LastRun()
				require.NoError(t, err)
				assert.True(t, lr.IsZero())
			},
		},
		"with enforced TLS and one of the configured endpoints not using TLS": {
			enforceTLS: true,
			conf: []byte(`
endpoints:
- url: https://foo.bar
- url: http://foo.bar
`),
			assert: func(t *testing.T, err error, _ *Provider) {
				t.Helper()

				require.Error(t, err)
				require.ErrorIs(t, err, heimdall.ErrConfiguration)
				assert.Contains(t, err.Error(), "'endpoints'[1].'url' scheme must be https")
			},
		},
		"with two endpoints and watch interval configured": {
			conf: []byte(`
watch_interval: 5m
endpoints:
- url: http://foo.bar
- url: http://bar.foo
`),
			assert: func(t *testing.T, err error, prov *Provider) {
				t.Helper()

				require.NoError(t, err)
				require.NotNil(t, prov)
				assert.NotNil(t, prov.s)
				assert.NotNil(t, prov.p)
				assert.NotNil(t, prov.cancel)

				time.Sleep(250 * time.Millisecond)

				jobs := prov.s.Jobs()
				assert.Len(t, jobs, 2)

				lr, err := jobs[0].LastRun()
				require.NoError(t, err)
				assert.True(t, lr.IsZero())
				lr, err = jobs[1].LastRun()
				require.NoError(t, err)
				assert.True(t, lr.IsZero())
			},
		},
	} {
		t.Run(uc, func(t *testing.T) {
			// GIVEN
			providerConf, err := testsupport.DecodeTestConfig(tc.conf)
			require.NoError(t, err)

			es := config.EnforcementSettings{EnforceEgressTLS: tc.enforceTLS}
			validator, err := validation.NewValidator(
				validation.WithTagValidator(es),
				validation.WithErrorTranslator(es),
			)
			require.NoError(t, err)

			conf := &config.Configuration{
				Providers: config.RuleProviders{HTTPEndpoint: providerConf},
			}

			cch, err := memory.NewCache(nil, nil)
			require.NoError(t, err)

			appCtx := app.NewContextMock(t)
			appCtx.EXPECT().Logger().Return(log.Logger)
			appCtx.EXPECT().Config().Return(conf)
			appCtx.EXPECT().Validator().Maybe().Return(validator)

			// WHEN
			prov, err := NewProvider(appCtx, mocks.NewRuleSetProcessorMock(t), cch)

			// THEN
			tc.assert(t, err, prov)
		})
	}
}

func TestProviderLifecycle(t *testing.T) {
	t.Parallel()

	type ResponseWriter func(t *testing.T, w http.ResponseWriter)

	var (
		writeResponse ResponseWriter
		requestCount  int
		rcm           sync.Mutex
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rcm.Lock()
		requestCount++
		rcm.Unlock()

		writeResponse(t, w)
	}))

	defer srv.Close()

	for uc, tc := range map[string]struct {
		conf           []byte
		setupProcessor func(t *testing.T, processor *mocks.RuleSetProcessorMock)
		writeResponse  ResponseWriter
		assert         func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock)
	}{
		"with rule set loading error due server error response": {
			conf: []byte(`
endpoints:
- url: ` + srv.URL + `
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.WriteHeader(http.StatusBadRequest)
			},
			assert: func(t *testing.T, logs fmt.Stringer, _ *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(250 * time.Millisecond)

				messages := logs.String()
				assert.Contains(t, messages, "response code: 400")
				assert.Contains(t, messages, "No updates received")
			},
		},
		"with empty server response": {
			conf: []byte(`
endpoints:
- url: ` + srv.URL + `
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.WriteHeader(http.StatusOK)
			},
			assert: func(t *testing.T, logs fmt.Stringer, _ *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(250 * time.Millisecond)

				assert.Equal(t, 1, requestCount)
				assert.Contains(t, logs.String(), "No updates received")
			},
		},
		"with not empty server response and without watch interval": {
			conf: []byte(`
endpoints:
- url: ` + srv.URL + `
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.Header().Set("Content-Type", "application/yaml")
				_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: foo
  match:
    routes:
      - path: /foo
    methods: [ "GET" ]
  execute:
    - authenticator: test
`))
				require.NoError(t, err)
			},
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Capture).
					Return(nil).Once()
			},
			assert: func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(600 * time.Millisecond)

				assert.Equal(t, 1, requestCount)
				assert.NotContains(t, logs.String(), "No updates received")

				_, ruleSet := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Value()
				assert.Contains(t, ruleSet.Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSet.Version)
				assert.Equal(t, "test", ruleSet.Name)
				assert.Len(t, ruleSet.Rules, 1)
				assert.Equal(t, "foo", ruleSet.Rules[0].ID)
			},
		},
		"with not empty server response and with watch interval": {
			conf: []byte(`
watch_interval: 250ms
endpoints:
  - url: ` + srv.URL + `
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.Header().Set("Content-Type", "application/yaml")
				_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
    methods: [ "GET" ]
  execute:
    - authenticator: test
`))
				require.NoError(t, err)
			},
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Capture).
					Return(nil).Once()
			},
			assert: func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(600 * time.Millisecond)

				assert.Equal(t, 3, requestCount)
				assert.Contains(t, logs.String(), "No updates received")

				_, ruleSet := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Value()
				assert.Contains(t, ruleSet.Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSet.Version)
				assert.Equal(t, "test", ruleSet.Name)
				assert.Len(t, ruleSet.Rules, 1)
				assert.Equal(t, "bar", ruleSet.Rules[0].ID)
			},
		},
		"first request successful, second request with 500, successive requests successful without changes": {
			conf: []byte(`
watch_interval: 250ms
endpoints:
  - url: ` + srv.URL + `
`),
			writeResponse: func() ResponseWriter {
				callIdx := 1

				return func(t *testing.T, w http.ResponseWriter) {
					t.Helper()

					switch callIdx {
					case 1:
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: foo
  match:
    routes:
      - path: /foo
    methods: [ GET ]
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					case 2:
						w.WriteHeader(http.StatusNotFound)
					default:
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "2"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
    methods: [ GET ]
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					}

					callIdx++
				}
			}(),
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Capture).
					Return(nil).Twice()

				processor.EXPECT().OnDeleted(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor2").Capture).
					Return(nil).Once()
			},
			assert: func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(1000 * time.Millisecond)

				assert.GreaterOrEqual(t, requestCount, 4)
				assert.Contains(t, logs.String(), "No updates received")

				_, ruleSets := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Values()
				assert.Contains(t, ruleSets[0].Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSets[0].Version)
				assert.Equal(t, "test", ruleSets[0].Name)
				assert.Len(t, ruleSets[0].Rules, 1)
				assert.Equal(t, "foo", ruleSets[0].Rules[0].ID)

				assert.Contains(t, ruleSets[1].Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "2", ruleSets[1].Version)
				assert.Equal(t, "test", ruleSets[1].Name)
				assert.Len(t, ruleSets[1].Rules, 1)
				assert.Equal(t, "bar", ruleSets[1].Rules[0].ID)

				_, ruleSet := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor2").Value()
				assert.Contains(t, ruleSet.Source, "http_endpoint:"+srv.URL)
				assert.Empty(t, ruleSet.Rules)
			},
		},
		"successive changes to the rule set in each retrieval": {
			conf: []byte(`
watch_interval: 200ms
endpoints:
  - url: ` + srv.URL + `
`),
			writeResponse: func() ResponseWriter {
				callIdx := 1

				return func(t *testing.T, w http.ResponseWriter) {
					t.Helper()

					switch callIdx {
					case 1:
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					case 2:
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: baz
  match:
    routes: 
      - path: /baz
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					case 3:
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: foo
  match:
    routes:
      - path: /foo
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					default:
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: foz
  match:
    routes:
      - path: /foz
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					}

					callIdx++
				}
			}(),
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Capture).
					Return(nil).Once()

				processor.EXPECT().OnUpdated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor2").Capture).
					Return(nil).Times(3)
			},
			assert: func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(2 * time.Second)

				assert.GreaterOrEqual(t, requestCount, 4)
				assert.Contains(t, logs.String(), "No updates received")

				_, ruleSet := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Value()
				assert.Contains(t, ruleSet.Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSet.Version)
				assert.Equal(t, "test", ruleSet.Name)
				assert.Len(t, ruleSet.Rules, 1)
				assert.Equal(t, "bar", ruleSet.Rules[0].ID)

				_, ruleSets := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor2").Values()
				assert.Contains(t, ruleSets[0].Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSets[0].Version)
				assert.Equal(t, "test", ruleSets[0].Name)
				assert.Len(t, ruleSets[0].Rules, 1)
				assert.Equal(t, "baz", ruleSets[0].Rules[0].ID)

				assert.Contains(t, ruleSets[1].Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSets[1].Version)
				assert.Equal(t, "test", ruleSets[1].Name)
				assert.Len(t, ruleSets[1].Rules, 1)
				assert.Equal(t, "foo", ruleSets[1].Rules[0].ID)

				assert.Contains(t, ruleSets[2].Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSets[2].Version)
				assert.Equal(t, "test", ruleSets[2].Name)
				assert.Len(t, ruleSets[2].Rules, 1)
				assert.Equal(t, "foz", ruleSets[2].Rules[0].ID)
			},
		},
		"response is cached": {
			conf: []byte(`
watch_interval: 250ms
endpoints:
  - url: ` + srv.URL + `
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.Header().Set("Expires", time.Now().Add(20*time.Second).UTC().Format(http.TimeFormat))
				w.Header().Set("Content-Type", "application/yaml")
				_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
  execute:
    - authenticator: test
`))
				require.NoError(t, err)
			},
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Capture).
					Return(nil).Once()
			},
			assert: func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(1 * time.Second)

				assert.Equal(t, 1, requestCount)
				assert.GreaterOrEqual(t, strings.Count(logs.String(), "No updates received"), 3)

				_, ruleSet := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Value()
				assert.Contains(t, ruleSet.Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSet.Version)
				assert.Equal(t, "test", ruleSet.Name)
				assert.Len(t, ruleSet.Rules, 1)
				assert.Equal(t, "bar", ruleSet.Rules[0].ID)
			},
		},
		"response is not cached, as caching is disabled": {
			conf: []byte(`
watch_interval: 250ms
endpoints:
  - url: ` + srv.URL + `
    http_cache: 
      enabled: false
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.Header().Set("Expires", time.Now().Add(20*time.Second).UTC().Format(http.TimeFormat))
				w.Header().Set("Content-Type", "application/yaml")
				_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
  execute:
    - authenticator: test
`))
				require.NoError(t, err)
			},
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).
					Run(mock2.NewArgumentCaptor2[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Capture).
					Return(nil).Once()
			},
			assert: func(t *testing.T, logs fmt.Stringer, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(1 * time.Second)

				assert.GreaterOrEqual(t, requestCount, 4)

				noUpdatesCount := strings.Count(logs.String(), "No updates received")
				assert.GreaterOrEqual(t, noUpdatesCount, 3)

				_, ruleSet := mock2.ArgumentCaptor2From[context.Context, *config2.RuleSet](&processor.Mock, "captor1").Value()
				assert.Contains(t, ruleSet.Source, "http_endpoint:"+srv.URL)
				assert.Equal(t, "1", ruleSet.Version)
				assert.Equal(t, "test", ruleSet.Name)
				assert.Len(t, ruleSet.Rules, 1)
				assert.Equal(t, "bar", ruleSet.Rules[0].ID)
			},
		},
		"previously unknown rule set with error on creation": {
			conf: []byte(`
endpoints:
- url: ` + srv.URL + `
`),
			writeResponse: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()

				w.Header().Set("Content-Type", "application/yaml")
				_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: foo
  match:
    routes:
      - path: /foo
  execute:
    - authenticator: test
`))
				require.NoError(t, err)
			},
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).Return(errors.New("test error")).Once()
			},
			assert: func(t *testing.T, logs fmt.Stringer, _ *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(200 * time.Millisecond)

				assert.Equal(t, 1, requestCount)
				assert.Contains(t, logs.String(), "Failed to apply rule set changes")
			},
		},
		"updated rule set with error on update": {
			conf: []byte(`
watch_interval: 200ms
endpoints:
- url: ` + srv.URL + `
`),
			writeResponse: func() ResponseWriter {
				callIdx := 1

				return func(t *testing.T, w http.ResponseWriter) {
					t.Helper()

					if callIdx == 1 {
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					} else {
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: baz
  match:
    routes:
      - path: /baz
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					}

					callIdx++
				}
			}(),
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				processor.EXPECT().OnCreated(mock.Anything, mock.Anything).Return(nil).Once()
				processor.EXPECT().OnUpdated(mock.Anything, mock.Anything).Return(errors.New("test error"))
			},
			assert: func(t *testing.T, logs fmt.Stringer, _ *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(1 * time.Second)

				assert.GreaterOrEqual(t, requestCount, 2)
				assert.Contains(t, logs.String(), "Failed to apply rule set changes")
			},
		},
		"deleted rule set with error on delete": {
			conf: []byte(`
watch_interval: 200ms
endpoints:
- url: ` + srv.URL + `
`),
			writeResponse: func() ResponseWriter {
				callIdx := 1

				return func(t *testing.T, w http.ResponseWriter) {
					t.Helper()

					if callIdx == 1 {
						w.Header().Set("Content-Type", "application/yaml")
						_, err := w.Write([]byte(`
version: "1"
name: test
rules:
- id: bar
  match:
    routes:
      - path: /bar
  execute:
    - authenticator: test
`))
						require.NoError(t, err)
					} else {
						w.WriteHeader(http.StatusNotFound)
					}

					callIdx++
				}
			}(),
			setupProcessor: func(t *testing.T, processor *mocks.RuleSetProcessorMock) {
				t.Helper()

				call := processor.EXPECT().OnCreated(mock.Anything, mock.Anything).Return(nil).Once()
				processor.EXPECT().OnDeleted(mock.Anything, mock.Anything).Return(errors.New("test error")).NotBefore(call)
			},
			assert: func(t *testing.T, logs fmt.Stringer, _ *mocks.RuleSetProcessorMock) {
				t.Helper()

				time.Sleep(1 * time.Second)

				assert.GreaterOrEqual(t, requestCount, 2)
				assert.Contains(t, logs.String(), "Failed to apply rule set changes")
			},
		},
	} {
		t.Run(uc, func(t *testing.T) {
			// GIVEN
			requestCount = 0

			setupProcessor := x.IfThenElse(tc.setupProcessor != nil,
				tc.setupProcessor,
				func(t *testing.T, _ *mocks.RuleSetProcessorMock) { t.Helper() })

			providerConf, err := testsupport.DecodeTestConfig(tc.conf)
			require.NoError(t, err)

			conf := &config.Configuration{
				Providers: config.RuleProviders{HTTPEndpoint: providerConf},
			}

			processor := mocks.NewRuleSetProcessorMock(t)
			setupProcessor(t, processor)

			logs := &strings.Builder{}

			cch, err := memory.NewCache(nil, nil)
			require.NoError(t, err)

			validator, err := validation.NewValidator(
				validation.WithTagValidator(config.EnforcementSettings{}),
			)
			require.NoError(t, err)

			appCtx := app.NewContextMock(t)
			appCtx.EXPECT().Logger().Return(zerolog.New(logs))
			appCtx.EXPECT().Config().Return(conf)
			appCtx.EXPECT().Validator().Return(validator)

			prov, err := NewProvider(appCtx, processor, cch)
			require.NoError(t, err)

			ctx := t.Context()

			writeResponse = x.IfThenElse(tc.writeResponse != nil,
				tc.writeResponse,
				func(t *testing.T, w http.ResponseWriter) {
					t.Helper()

					w.WriteHeader(http.StatusOK)
				})

			// WHEN
			err = prov.Start(ctx)

			defer prov.Stop(ctx) //nolint:errcheck

			// time.Sleep(2000 * time.Millisecond)

			// THEN
			require.NoError(t, err)
			tc.assert(t, logs, processor)
		})
	}
}
