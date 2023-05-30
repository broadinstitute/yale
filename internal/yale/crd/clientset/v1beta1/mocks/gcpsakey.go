// Code generated by mockery v2.26.1. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1beta1 "github.com/broadinstitute/yale/internal/yale/crd/api/v1beta1"
)

// GcpSaKeyInterface is an autogenerated mock type for the GcpSaKeyInterface type
type GcpSaKeyInterface struct {
	mock.Mock
}

type GcpSaKeyInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *GcpSaKeyInterface) EXPECT() *GcpSaKeyInterface_Expecter {
	return &GcpSaKeyInterface_Expecter{mock: &_m.Mock}
}

// Get provides a mock function with given fields: ctx, name, options
func (_m *GcpSaKeyInterface) Get(ctx context.Context, name string, options v1.GetOptions) (*v1beta1.GcpSaKey, error) {
	ret := _m.Called(ctx, name, options)

	var r0 *v1beta1.GcpSaKey
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, v1.GetOptions) (*v1beta1.GcpSaKey, error)); ok {
		return rf(ctx, name, options)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, v1.GetOptions) *v1beta1.GcpSaKey); ok {
		r0 = rf(ctx, name, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1beta1.GcpSaKey)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, v1.GetOptions) error); ok {
		r1 = rf(ctx, name, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GcpSaKeyInterface_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type GcpSaKeyInterface_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - ctx context.Context
//   - name string
//   - options v1.GetOptions
func (_e *GcpSaKeyInterface_Expecter) Get(ctx interface{}, name interface{}, options interface{}) *GcpSaKeyInterface_Get_Call {
	return &GcpSaKeyInterface_Get_Call{Call: _e.mock.On("Get", ctx, name, options)}
}

func (_c *GcpSaKeyInterface_Get_Call) Run(run func(ctx context.Context, name string, options v1.GetOptions)) *GcpSaKeyInterface_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(v1.GetOptions))
	})
	return _c
}

func (_c *GcpSaKeyInterface_Get_Call) Return(_a0 *v1beta1.GcpSaKey, _a1 error) *GcpSaKeyInterface_Get_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *GcpSaKeyInterface_Get_Call) RunAndReturn(run func(context.Context, string, v1.GetOptions) (*v1beta1.GcpSaKey, error)) *GcpSaKeyInterface_Get_Call {
	_c.Call.Return(run)
	return _c
}

// List provides a mock function with given fields: ctx, opts
func (_m *GcpSaKeyInterface) List(ctx context.Context, opts v1.ListOptions) (*v1beta1.GCPSaKeyList, error) {
	ret := _m.Called(ctx, opts)

	var r0 *v1beta1.GCPSaKeyList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) (*v1beta1.GCPSaKeyList, error)); ok {
		return rf(ctx, opts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, v1.ListOptions) *v1beta1.GCPSaKeyList); ok {
		r0 = rf(ctx, opts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1beta1.GCPSaKeyList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, v1.ListOptions) error); ok {
		r1 = rf(ctx, opts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GcpSaKeyInterface_List_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'List'
type GcpSaKeyInterface_List_Call struct {
	*mock.Call
}

// List is a helper method to define mock.On call
//   - ctx context.Context
//   - opts v1.ListOptions
func (_e *GcpSaKeyInterface_Expecter) List(ctx interface{}, opts interface{}) *GcpSaKeyInterface_List_Call {
	return &GcpSaKeyInterface_List_Call{Call: _e.mock.On("List", ctx, opts)}
}

func (_c *GcpSaKeyInterface_List_Call) Run(run func(ctx context.Context, opts v1.ListOptions)) *GcpSaKeyInterface_List_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(v1.ListOptions))
	})
	return _c
}

func (_c *GcpSaKeyInterface_List_Call) Return(_a0 *v1beta1.GCPSaKeyList, _a1 error) *GcpSaKeyInterface_List_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *GcpSaKeyInterface_List_Call) RunAndReturn(run func(context.Context, v1.ListOptions) (*v1beta1.GCPSaKeyList, error)) *GcpSaKeyInterface_List_Call {
	_c.Call.Return(run)
	return _c
}

type mockConstructorTestingTNewGcpSaKeyInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewGcpSaKeyInterface creates a new instance of GcpSaKeyInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewGcpSaKeyInterface(t mockConstructorTestingTNewGcpSaKeyInterface) *GcpSaKeyInterface {
	mock := &GcpSaKeyInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
