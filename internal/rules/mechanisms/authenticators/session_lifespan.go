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

package authenticators

import (
	"errors"
	"time"

	"github.com/dadrus/heimdall/internal/x"
	"github.com/dadrus/heimdall/internal/x/errorchain"
)

var ErrSessionValidity = errors.New("session validity error")

const defaultLeeway = 10 * time.Second

type SessionLifespan struct {
	active bool
	iat    time.Time
	nbf    time.Time
	exp    time.Time
	leeway time.Duration
}

func (s *SessionLifespan) Assert() error {
	if !s.active {
		return errorchain.NewWithMessage(ErrSessionValidity, "not active")
	}

	if err := s.assertValidity(); err != nil {
		return err
	}

	return s.assertIssuanceTime()
}

func (s *SessionLifespan) assertValidity() error {
	leeway := int64(x.IfThenElse(s.leeway != 0, s.leeway, defaultLeeway).Seconds())
	now := time.Now().Unix()
	nbf := s.nbf.Unix()
	exp := s.exp.Unix()

	if nbf > 0 && now+leeway < nbf {
		return errorchain.NewWithMessage(ErrSessionValidity, "not yet valid")
	}

	if exp > 0 && now-leeway >= exp {
		return errorchain.NewWithMessage(ErrSessionValidity, "expired")
	}

	return nil
}

func (s *SessionLifespan) assertIssuanceTime() error {
	leeway := x.IfThenElse(s.leeway != 0, s.leeway, defaultLeeway)

	// IssuedAt is optional but cannot be in the future. This is not required by the RFC, but
	// if by misconfiguration it has been set to future, we don't trust it.
	if !s.iat.Equal(time.Time{}) && time.Now().Add(leeway).Before(s.iat) {
		return errorchain.NewWithMessage(ErrSessionValidity, "issued in the future")
	}

	return nil
}
