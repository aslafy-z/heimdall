// Code generated by mockery v2.42.1. DO NOT EDIT.

package mocks

import (
	certificate "github.com/dadrus/heimdall/internal/otel/metrics/certificate"
	mock "github.com/stretchr/testify/mock"
)

// ObserverMock is an autogenerated mock type for the Observer type
type ObserverMock struct {
	mock.Mock
}

type ObserverMock_Expecter struct {
	mock *mock.Mock
}

func (_m *ObserverMock) EXPECT() *ObserverMock_Expecter {
	return &ObserverMock_Expecter{mock: &_m.Mock}
}

// Add provides a mock function with given fields: sup
func (_m *ObserverMock) Add(sup certificate.Supplier) {
	_m.Called(sup)
}

// ObserverMock_Add_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Add'
type ObserverMock_Add_Call struct {
	*mock.Call
}

// Add is a helper method to define mock.On call
//   - sup certificate.Supplier
func (_e *ObserverMock_Expecter) Add(sup interface{}) *ObserverMock_Add_Call {
	return &ObserverMock_Add_Call{Call: _e.mock.On("Add", sup)}
}

func (_c *ObserverMock_Add_Call) Run(run func(sup certificate.Supplier)) *ObserverMock_Add_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(certificate.Supplier))
	})
	return _c
}

func (_c *ObserverMock_Add_Call) Return() *ObserverMock_Add_Call {
	_c.Call.Return()
	return _c
}

func (_c *ObserverMock_Add_Call) RunAndReturn(run func(certificate.Supplier)) *ObserverMock_Add_Call {
	_c.Call.Return(run)
	return _c
}

// Start provides a mock function with given fields:
func (_m *ObserverMock) Start() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Start")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ObserverMock_Start_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Start'
type ObserverMock_Start_Call struct {
	*mock.Call
}

// Start is a helper method to define mock.On call
func (_e *ObserverMock_Expecter) Start() *ObserverMock_Start_Call {
	return &ObserverMock_Start_Call{Call: _e.mock.On("Start")}
}

func (_c *ObserverMock_Start_Call) Run(run func()) *ObserverMock_Start_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *ObserverMock_Start_Call) Return(_a0 error) *ObserverMock_Start_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *ObserverMock_Start_Call) RunAndReturn(run func() error) *ObserverMock_Start_Call {
	_c.Call.Return(run)
	return _c
}

// NewObserverMock creates a new instance of ObserverMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewObserverMock(t interface {
	mock.TestingT
	Cleanup(func())
}) *ObserverMock {
	mock := &ObserverMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
