// Code generated by mockery v2.26.1. DO NOT EDIT.

package mocks

import (
	time "time"

	mock "github.com/stretchr/testify/mock"
)

// AuthMetrics is an autogenerated mock type for the AuthMetrics type
type AuthMetrics struct {
	mock.Mock
}

type AuthMetrics_Expecter struct {
	mock *mock.Mock
}

func (_m *AuthMetrics) EXPECT() *AuthMetrics_Expecter {
	return &AuthMetrics_Expecter{mock: &_m.Mock}
}

// LastAuthTime provides a mock function with given fields: project, serviceAccountEmail, keyID
func (_m *AuthMetrics) LastAuthTime(project string, serviceAccountEmail string, keyID string) (*time.Time, error) {
	ret := _m.Called(project, serviceAccountEmail, keyID)

	var r0 *time.Time
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string, string) (*time.Time, error)); ok {
		return rf(project, serviceAccountEmail, keyID)
	}
	if rf, ok := ret.Get(0).(func(string, string, string) *time.Time); ok {
		r0 = rf(project, serviceAccountEmail, keyID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*time.Time)
		}
	}

	if rf, ok := ret.Get(1).(func(string, string, string) error); ok {
		r1 = rf(project, serviceAccountEmail, keyID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// AuthMetrics_LastAuthTime_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'LastAuthTime'
type AuthMetrics_LastAuthTime_Call struct {
	*mock.Call
}

// LastAuthTime is a helper method to define mock.On call
//   - project string
//   - serviceAccountEmail string
//   - keyID string
func (_e *AuthMetrics_Expecter) LastAuthTime(project interface{}, serviceAccountEmail interface{}, keyID interface{}) *AuthMetrics_LastAuthTime_Call {
	return &AuthMetrics_LastAuthTime_Call{Call: _e.mock.On("LastAuthTime", project, serviceAccountEmail, keyID)}
}

func (_c *AuthMetrics_LastAuthTime_Call) Run(run func(project string, serviceAccountEmail string, keyID string)) *AuthMetrics_LastAuthTime_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *AuthMetrics_LastAuthTime_Call) Return(_a0 *time.Time, _a1 error) *AuthMetrics_LastAuthTime_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *AuthMetrics_LastAuthTime_Call) RunAndReturn(run func(string, string, string) (*time.Time, error)) *AuthMetrics_LastAuthTime_Call {
	_c.Call.Return(run)
	return _c
}

type mockConstructorTestingTNewAuthMetrics interface {
	mock.TestingT
	Cleanup(func())
}

// NewAuthMetrics creates a new instance of AuthMetrics. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewAuthMetrics(t mockConstructorTestingTNewAuthMetrics) *AuthMetrics {
	mock := &AuthMetrics{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
