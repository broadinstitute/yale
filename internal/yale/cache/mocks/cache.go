// Code generated by mockery v2.28.2. DO NOT EDIT.

package mocks

import (
	cache "github.com/broadinstitute/yale/internal/yale/cache"
	mock "github.com/stretchr/testify/mock"
)

// Cache is an autogenerated mock type for the Cache type
type Cache struct {
	mock.Mock
}

type Cache_Expecter struct {
	mock *mock.Mock
}

func (_m *Cache) EXPECT() *Cache_Expecter {
	return &Cache_Expecter{mock: &_m.Mock}
}

// Delete provides a mock function with given fields: _a0
func (_m *Cache) Delete(_a0 *cache.Entry) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(*cache.Entry) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Cache_Delete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Delete'
type Cache_Delete_Call struct {
	*mock.Call
}

// Delete is a helper method to define mock.On call
//   - _a0 *cache.Entry
func (_e *Cache_Expecter) Delete(_a0 interface{}) *Cache_Delete_Call {
	return &Cache_Delete_Call{Call: _e.mock.On("Delete", _a0)}
}

func (_c *Cache_Delete_Call) Run(run func(_a0 *cache.Entry)) *Cache_Delete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*cache.Entry))
	})
	return _c
}

func (_c *Cache_Delete_Call) Return(_a0 error) *Cache_Delete_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Cache_Delete_Call) RunAndReturn(run func(*cache.Entry) error) *Cache_Delete_Call {
	_c.Call.Return(run)
	return _c
}

// GetOrCreate provides a mock function with given fields: _a0
func (_m *Cache) GetOrCreate(_a0 cache.Identifier) (*cache.Entry, error) {
	ret := _m.Called(_a0)

	var r0 *cache.Entry
	var r1 error
	if rf, ok := ret.Get(0).(func(cache.Identifier) (*cache.Entry, error)); ok {
		return rf(_a0)
	}
	if rf, ok := ret.Get(0).(func(cache.Identifier) *cache.Entry); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*cache.Entry)
		}
	}

	if rf, ok := ret.Get(1).(func(cache.Identifier) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Cache_GetOrCreate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetOrCreate'
type Cache_GetOrCreate_Call struct {
	*mock.Call
}

// GetOrCreate is a helper method to define mock.On call
//   - _a0 cache.Identifier
func (_e *Cache_Expecter) GetOrCreate(_a0 interface{}) *Cache_GetOrCreate_Call {
	return &Cache_GetOrCreate_Call{Call: _e.mock.On("GetOrCreate", _a0)}
}

func (_c *Cache_GetOrCreate_Call) Run(run func(_a0 cache.Identifier)) *Cache_GetOrCreate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(cache.Identifier))
	})
	return _c
}

func (_c *Cache_GetOrCreate_Call) Return(_a0 *cache.Entry, _a1 error) *Cache_GetOrCreate_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Cache_GetOrCreate_Call) RunAndReturn(run func(cache.Identifier) (*cache.Entry, error)) *Cache_GetOrCreate_Call {
	_c.Call.Return(run)
	return _c
}

// List provides a mock function with given fields:
func (_m *Cache) List() ([]*cache.Entry, error) {
	ret := _m.Called()

	var r0 []*cache.Entry
	var r1 error
	if rf, ok := ret.Get(0).(func() ([]*cache.Entry, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() []*cache.Entry); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*cache.Entry)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Cache_List_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'List'
type Cache_List_Call struct {
	*mock.Call
}

// List is a helper method to define mock.On call
func (_e *Cache_Expecter) List() *Cache_List_Call {
	return &Cache_List_Call{Call: _e.mock.On("List")}
}

func (_c *Cache_List_Call) Run(run func()) *Cache_List_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Cache_List_Call) Return(_a0 []*cache.Entry, _a1 error) *Cache_List_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Cache_List_Call) RunAndReturn(run func() ([]*cache.Entry, error)) *Cache_List_Call {
	_c.Call.Return(run)
	return _c
}

// Save provides a mock function with given fields: _a0
func (_m *Cache) Save(_a0 *cache.Entry) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(*cache.Entry) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Cache_Save_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Save'
type Cache_Save_Call struct {
	*mock.Call
}

// Save is a helper method to define mock.On call
//   - _a0 *cache.Entry
func (_e *Cache_Expecter) Save(_a0 interface{}) *Cache_Save_Call {
	return &Cache_Save_Call{Call: _e.mock.On("Save", _a0)}
}

func (_c *Cache_Save_Call) Run(run func(_a0 *cache.Entry)) *Cache_Save_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*cache.Entry))
	})
	return _c
}

func (_c *Cache_Save_Call) Return(_a0 error) *Cache_Save_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Cache_Save_Call) RunAndReturn(run func(*cache.Entry) error) *Cache_Save_Call {
	_c.Call.Return(run)
	return _c
}

type mockConstructorTestingTNewCache interface {
	mock.TestingT
	Cleanup(func())
}

// NewCache creates a new instance of Cache. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewCache(t mockConstructorTestingTNewCache) *Cache {
	mock := &Cache{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
