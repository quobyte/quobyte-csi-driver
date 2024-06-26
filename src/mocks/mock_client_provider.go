// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/quobyte/quobyte-csi-driver/driver (interfaces: QuobyteApiClientProvider)
//
// Generated by this command:
//
//	mockgen -package=mocks -destination ../mocks/mock_client_provider.go github.com/quobyte/quobyte-csi-driver/driver QuobyteApiClientProvider
//

// Package mocks is a generated GoMock package.
package mocks

import (
	url "net/url"
	reflect "reflect"

	quobyte "github.com/quobyte/api/quobyte"
	gomock "go.uber.org/mock/gomock"
)

// MockQuobyteApiClientProvider is a mock of QuobyteApiClientProvider interface.
type MockQuobyteApiClientProvider struct {
	ctrl     *gomock.Controller
	recorder *MockQuobyteApiClientProviderMockRecorder
}

// MockQuobyteApiClientProviderMockRecorder is the mock recorder for MockQuobyteApiClientProvider.
type MockQuobyteApiClientProviderMockRecorder struct {
	mock *MockQuobyteApiClientProvider
}

// NewMockQuobyteApiClientProvider creates a new mock instance.
func NewMockQuobyteApiClientProvider(ctrl *gomock.Controller) *MockQuobyteApiClientProvider {
	mock := &MockQuobyteApiClientProvider{ctrl: ctrl}
	mock.recorder = &MockQuobyteApiClientProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockQuobyteApiClientProvider) EXPECT() *MockQuobyteApiClientProviderMockRecorder {
	return m.recorder
}

// NewQuobyteApiClient mocks base method.
func (m *MockQuobyteApiClientProvider) NewQuobyteApiClient(arg0 *url.URL, arg1 map[string]string) (quobyte.ExtendedQuobyteApi, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewQuobyteApiClient", arg0, arg1)
	ret0, _ := ret[0].(quobyte.ExtendedQuobyteApi)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewQuobyteApiClient indicates an expected call of NewQuobyteApiClient.
func (mr *MockQuobyteApiClientProviderMockRecorder) NewQuobyteApiClient(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewQuobyteApiClient", reflect.TypeOf((*MockQuobyteApiClientProvider)(nil).NewQuobyteApiClient), arg0, arg1)
}
