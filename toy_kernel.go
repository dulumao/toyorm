/*
 * Copyright 2018. bigpigeon. All rights reserved.
 * Use of this source code is governed by a MIT style
 * license that can be found in the LICENSE file.
 */

package toyorm

import (
	"io"
	"reflect"
)

type ToyKernel struct {
	CacheModels              map[reflect.Type]*Model
	CacheMiddleModels        map[reflect.Type]*Model
	CacheReverseMiddleModels map[reflect.Type]*Model
	debug                    bool
	// map[model][container_field_name]
	Dialect Dialect
	Logger  io.Writer
}

// TODO testing thread safe? if not add lock
func (t *ToyKernel) GetModel(_type reflect.Type) *Model {
	if model, ok := t.CacheModels[_type]; ok == false {
		model = NewModel(_type)
		t.CacheModels[_type] = model
	}
	return t.CacheModels[_type]
}

func (t *ToyKernel) GetMiddleModel(_type reflect.Type) *Model {
	if model, ok := t.CacheModels[_type]; ok == false {
		model = NewModel(_type)
		t.CacheModels[_type] = model
	}
	return t.CacheModels[_type]
}

func (t *ToyKernel) SetDebug(debug bool) {
	t.debug = debug
}
