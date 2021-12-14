package rpc_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gojek/turing/engines/experiment/plugin/rpc"
	mocks2 "github.com/gojek/turing/engines/experiment/plugin/rpc/mocks"
	"testing"

	"bou.ke/monkey"
	goPlugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

const (
	dispenseError = "unknown error"
	configError   = "config error"
)

func withPatchedConnect(client goPlugin.ClientProtocol, err string, fn func()) {
	monkey.Patch(rpc.Connect,
		func(pluginBinary string, logger *zap.Logger) (goPlugin.ClientProtocol, error) {
			if err != "" {
				return nil, errors.New(err)
			}
			return client, nil
		})
	defer monkey.Unpatch(rpc.Connect)

	fn()
}

func TestNewFactory(t *testing.T) {
	suite := map[string]struct {
		cfg json.RawMessage
		err string
	}{
		"success": {
			cfg: json.RawMessage("{\"key_1\": \"value_1\"}"),
		},
		"failure | connection failure": {
			err: "no plugin found",
		},
	}

	logger, _ := zap.NewDevelopment()
	for name, tt := range suite {
		mockClient := &mocks2.ClientProtocol{}

		t.Run(name, func(t *testing.T) {
			withPatchedConnect(mockClient, tt.err, func() {
				actual, err := rpc.NewFactory("path/to/plugin", tt.cfg, logger.Sugar())
				if tt.err != "" {
					assert.EqualError(t, err, tt.err)
					assert.Nil(t, actual)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, actual)
					assert.Same(t, mockClient, actual.Client)
					assert.Equal(t, tt.cfg, actual.EngineConfig)
				}
			})
		})
	}
}

func TestEngineFactory_GetExperimentManager(t *testing.T) {
	suite := map[string]struct {
		cfg            json.RawMessage
		mockManager    func(json.RawMessage) interface{}
		failToDispense bool
		failToConfig   bool
		err            string
	}{
		"success": {
			cfg: json.RawMessage("{\"key_1\": \"value_1\"}"),
			mockManager: func(cfg json.RawMessage) interface{} {
				mockManager := &mocks2.ConfigurableExperimentManager{}
				mockManager.
					On("Configure", cfg).
					Return(func() error { return nil })

				return mockManager
			},
		},
		"failure | failed to dispense plugin": {
			mockManager: func(json.RawMessage) interface{} {
				return nil
			},
			failToDispense: true,
			err: fmt.Sprintf(
				"unable to retrieve \"%s\" plugin instance: %v",
				rpc.ManagerPluginIdentifier, dispenseError),
		},
		"failure | plugin doesn't meet interface requirements": {
			mockManager: func(json.RawMessage) interface{} {
				return new(interface{})
			},
			err: fmt.Sprintf(
				"unable to cast *interface {} to shared.Configurable for plugin \"%s\"",
				rpc.ManagerPluginIdentifier),
		},
		"failure | failed to configure plugin": {
			mockManager: func(cfg json.RawMessage) interface{} {
				mockManager := &mocks2.ConfigurableExperimentManager{}
				mockManager.
					On("Configure", cfg).
					Return(func() error {
						return errors.New(configError)
					})

				return mockManager
			},
			failToConfig: true,
			err: fmt.Sprintf(
				"failed to configure \"experiment_manager\" plugin instance: %v", configError),
		},
	}

	logger, _ := zap.NewDevelopment()
	for name, tt := range suite {
		t.Run(name, func(t *testing.T) {
			mockManager := tt.mockManager(tt.cfg)

			mockClient := &mocks2.ClientProtocol{}
			mockClient.On("Dispense", rpc.ManagerPluginIdentifier).
				Return(mockManager,
					func(string) error {
						if tt.failToDispense {
							return errors.New(dispenseError)
						}
						return nil
					},
				).Once()

			withPatchedConnect(mockClient, "", func() {
				factory, _ := rpc.NewFactory("path/to/plugin", tt.cfg, logger.Sugar())
				actual, err := factory.GetExperimentManager()

				if tt.err != "" {
					assert.EqualError(t, err, tt.err)
					assert.Nil(t, actual)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, actual)
					assert.Same(t, mockManager, actual)

					another, err := factory.GetExperimentManager()
					assert.NoError(t, err)
					assert.Same(t, actual, another)

					mockManager.(*mocks2.ConfigurableExperimentManager).AssertExpectations(t)
				}
			})

			mockClient.AssertExpectations(t)
		})
	}
}