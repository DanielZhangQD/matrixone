// Copyright 2022 Matrix Origin
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

package aggut

import (
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/agg"
	"testing"

	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/container/vector"
	"github.com/matrixorigin/matrixone/pkg/testutil"
	"github.com/matrixorigin/matrixone/pkg/vm/mheap"
	"github.com/matrixorigin/matrixone/pkg/vm/mmu/guest"
	"github.com/matrixorigin/matrixone/pkg/vm/mmu/host"
	"github.com/stretchr/testify/require"
)

// TODO: add distinc decimal128 test
func TestMin(t *testing.T) {
	testTyp := types.New(types.T_int64, 0, 0, 0)
	mn := agg.NewMin[int64]()
	mn2 := agg.NewMin[int64]()
	mn3 := agg.NewMin[int64]()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	vs := []int64{0, 1, -2, 3, 14, 5, -6, 7, 8, 9}
	vs2 := []int64{0, 1, -2, 3, -14, 5, -6, 7, 8, 29}
	vec := testutil.NewVector(Rows, testTyp, m, false, vs)
	vec2 := testutil.NewVector(Rows, testTyp, m, false, vs2)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryAgg(nil, true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill, nil)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, []int64{-6}, vector.GetColumn[int64](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, mn2.Grows, mn2.Eval, mn2.Merge, mn2.Fill, nil)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, mn3.Grows, mn3.Eval, mn3.Merge, mn3.Fill, nil)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []int64{-14}, vector.GetColumn[int64](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []int64{-14}, vector.GetColumn[int64](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestDecimalMin(t *testing.T) {
	testTyp := types.New(types.T_decimal128, 0, 0, 0)
	dmn := agg.NewD128Min()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	input1 := []int64{10, 1, 12, 3, 4, 5, 26, 7, 8, 9}
	input2 := []int64{0, 1, 2, 3, 14, 5, 6, 7, 8, 29}
	vec := testutil.MakeDecimal128Vector(input1, nil, testTyp)
	vec2 := testutil.MakeDecimal128Vector(input2, nil, testTyp)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryAgg(nil, true, testTyp, testTyp, dmn.Grows, dmn.Eval, dmn.Merge, dmn.Fill, nil)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, testutil.MakeDecimal128ArrByInt64Arr([]int64{1}), vector.GetColumn[types.Decimal128](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, dmn.Grows, dmn.Eval, dmn.Merge, dmn.Fill, nil)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, dmn.Grows, dmn.Eval, dmn.Merge, dmn.Fill, nil)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, testutil.MakeDecimal128ArrByInt64Arr([]int64{0}), vector.GetColumn[types.Decimal128](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, testutil.MakeDecimal128ArrByInt64Arr([]int64{0}), vector.GetColumn[types.Decimal128](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestBoollMin(t *testing.T) {
	testTyp := types.New(types.T_decimal128, 0, 0, 0)
	dmn := agg.NewBoolMin()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	input1 := []bool{false, true, false, true, false, true, false, true, false, true}
	input2 := []bool{true, true, true, true, true, true, true, true, true, true}
	vec := testutil.MakeBoolVector(input1)
	vec2 := testutil.MakeBoolVector(input2)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryAgg(nil, true, testTyp, testTyp, dmn.Grows, dmn.Eval, dmn.Merge, dmn.Fill, nil)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, []bool{false}, vector.GetColumn[bool](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, dmn.Grows, dmn.Eval, dmn.Merge, dmn.Fill, nil)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, dmn.Grows, dmn.Eval, dmn.Merge, dmn.Fill, nil)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []bool{false}, vector.GetColumn[bool](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []bool{true}, vector.GetColumn[bool](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestStrlMin(t *testing.T) {
	testTyp := types.New(types.T_varchar, 0, 0, 0)
	smn := agg.NewStrMin()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	input1 := []string{"ab", "ac", "bc", "bcdd", "c", "a", "mo", "momo", "zb", "z"}
	input2 := []string{"ab", "ac", "bc", "bcdd", "c", "za", "mo", "momo", "zb", "zzz"}
	vec := testutil.MakeVarcharVector(input1, nil)
	vec2 := testutil.MakeVarcharVector(input2, nil)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryAgg(nil, true, testTyp, testTyp, smn.Grows, smn.Eval, smn.Merge, smn.Fill, nil)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, makeBytes([]string{"a"}), vector.GetStrColumn(v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, smn.Grows, smn.Eval, smn.Merge, smn.Fill, nil)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, smn.Grows, smn.Eval, smn.Merge, smn.Fill, nil)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, makeBytes([]string{"a"}), vector.GetStrColumn(v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, makeBytes([]string{"ab"}), vector.GetStrColumn(v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestDistincMin(t *testing.T) {
	testTyp := types.New(types.T_int64, 0, 0, 0)
	mx := agg.NewMin[int64]()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	vs := []int64{0, 1, -2, 3, 14, 5, -6, 7, 8, 9}
	vs2 := []int64{0, 1, -2, 3, -14, 5, -6, 7, 8, 29}
	vec := testutil.NewVector(Rows, testTyp, m, false, vs)
	vec2 := testutil.NewVector(Rows, testTyp, m, false, vs2)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryDistAgg(true, testTyp, testTyp, mx.Grows, mx.Eval, mx.Merge, mx.Fill)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, []int64{-6}, vector.GetColumn[int64](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryDistAgg(true, testTyp, testTyp, mx.Grows, mx.Eval, mx.Merge, mx.Fill)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryDistAgg(true, testTyp, testTyp, mx.Grows, mx.Eval, mx.Merge, mx.Fill)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []int64{-14}, vector.GetColumn[int64](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []int64{-14}, vector.GetColumn[int64](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestDisctincBoollMin(t *testing.T) {
	testTyp := types.New(types.T_decimal128, 0, 0, 0)
	dmx := agg.NewBoolMin()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	input1 := []bool{false, true, false, true, false, true, false, true, false, true}
	input2 := []bool{true, true, true, true, true, true, true, true, true, true}
	vec := testutil.MakeBoolVector(input1)
	vec2 := testutil.MakeBoolVector(input2)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryDistAgg(true, testTyp, testTyp, dmx.Grows, dmx.Eval, dmx.Merge, dmx.Fill)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, []bool{false}, vector.GetColumn[bool](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryDistAgg(true, testTyp, testTyp, dmx.Grows, dmx.Eval, dmx.Merge, dmx.Fill)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryDistAgg(true, testTyp, testTyp, dmx.Grows, dmx.Eval, dmx.Merge, dmx.Fill)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []bool{false}, vector.GetColumn[bool](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, []bool{true}, vector.GetColumn[bool](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestDiscincStrlMin(t *testing.T) {
	testTyp := types.New(types.T_varchar, 0, 0, 0)
	smn := agg.NewStrMin()
	m := mheap.New(guest.New(1<<30, host.New(1<<30)))
	input1 := []string{"ab", "ab", "ab", "bcdd", "c", "a", "mo", "momo", "a", "z"}
	input2 := []string{"ab", "ac", "mo", "bcdd", "c", "mo", "mo", "ab", "zb", "zzz"}
	vec := testutil.MakeVarcharVector(input1, nil)
	vec2 := testutil.MakeVarcharVector(input2, nil)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryDistAgg(true, testTyp, testTyp, smn.Grows, smn.Eval, smn.Merge, smn.Fill)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)
		require.Equal(t, makeBytes([]string{"a"}), vector.GetStrColumn(v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryDistAgg(true, testTyp, testTyp, smn.Grows, smn.Eval, smn.Merge, smn.Fill)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryDistAgg(true, testTyp, testTyp, smn.Grows, smn.Eval, smn.Merge, smn.Fill)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			require.Equal(t, makeBytes([]string{"a"}), vector.GetStrColumn(v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			require.Equal(t, makeBytes([]string{"ab"}), vector.GetStrColumn(v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestUuidMin(t *testing.T) {
	testTyp := types.New(types.T_uuid, 0, 0, 0)
	mn := agg.NewUuidMin()

	m := mheap.New(guest.New(1<<30, host.New(1<<30)))

	vs := []string{
		"f6355110-2d0c-11ed-940f-000c29847904",
		"1ef96142-2d0d-11ed-940f-000c29847904",
		"117a0bd5-2d0d-11ed-940f-000c29847904",
		"18b21c70-2d0d-11ed-940f-000c29847904",
		"1b50c129-2dba-11ed-940f-000c29847904",
		"ad9f83eb-2dbd-11ed-940f-000c29847904",
		"6d1b1fdb-2dbf-11ed-940f-000c29847904",
		"6d1b1fdb-2dbf-11ed-940f-000c29847904",
		"1b50c129-2dba-11ed-940f-000c29847904",
		"ad9f83eb-2dbd-11ed-940f-000c29847904",
	}
	vs2 := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"3e350a5c-222a-11eb-abef-0242ac110002",
		"9e7862b3-2f69-11ed-8ec0-000c29847904",
		"6d1b1f73-2dbf-11ed-940f-000c29847904",
		"ad9f809f-2dbd-11ed-940f-000c29847904",
		"1b50c137-2dba-11ed-940f-000c29847904",
		"149e3f0f-2de4-11ed-940f-000c29847904",
		"1b50c137-2dba-11ed-940f-000c29847904",
		"9e7862b3-2f69-11ed-8ec0-000c29847904",
		"3F2504E0-4F89-11D3-9A0C-0305E82C3301",
	}
	vec := testutil.MakeUuidVectorByString(vs, nil)
	vec2 := testutil.MakeUuidVectorByString(vs2, nil)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryAgg(nil, true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill, nil)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)

		want, err := types.ParseUuid("117a0bd5-2d0d-11ed-940f-000c29847904")
		require.NoError(t, err)

		require.Equal(t, []types.Uuid{want}, vector.GetColumn[types.Uuid](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill, nil)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryAgg(nil, true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill, nil)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			want, err := types.ParseUuid("117a0bd5-2d0d-11ed-940f-000c29847904")
			require.NoError(t, err)

			require.Equal(t, []types.Uuid{want}, vector.GetColumn[types.Uuid](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			want, err := types.ParseUuid("149e3f0f-2de4-11ed-940f-000c29847904")
			require.NoError(t, err)

			require.Equal(t, []types.Uuid{want}, vector.GetColumn[types.Uuid](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func TestUuidDiscincMin(t *testing.T) {
	testTyp := types.New(types.T_uuid, 0, 0, 0)
	mn := agg.NewUuidMin()

	m := mheap.New(guest.New(1<<30, host.New(1<<30)))

	vs := []string{
		"f6355110-2d0c-11ed-940f-000c29847904",
		"1ef96142-2d0d-11ed-940f-000c29847904",
		"117a0bd5-2d0d-11ed-940f-000c29847904",
		"18b21c70-2d0d-11ed-940f-000c29847904",
		"1b50c129-2dba-11ed-940f-000c29847904",
		"ad9f83eb-2dbd-11ed-940f-000c29847904",
		"6d1b1fdb-2dbf-11ed-940f-000c29847904",
		"6d1b1fdb-2dbf-11ed-940f-000c29847904",
		"1b50c129-2dba-11ed-940f-000c29847904",
		"ad9f83eb-2dbd-11ed-940f-000c29847904",
	}
	vs2 := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"3e350a5c-222a-11eb-abef-0242ac110002",
		"9e7862b3-2f69-11ed-8ec0-000c29847904",
		"6d1b1f73-2dbf-11ed-940f-000c29847904",
		"ad9f809f-2dbd-11ed-940f-000c29847904",
		"1b50c137-2dba-11ed-940f-000c29847904",
		"149e3f0f-2de4-11ed-940f-000c29847904",
		"1b50c137-2dba-11ed-940f-000c29847904",
		"9e7862b3-2f69-11ed-8ec0-000c29847904",
		"3F2504E0-4F89-11D3-9A0C-0305E82C3301",
	}
	vec := testutil.MakeUuidVectorByString(vs, nil)
	vec2 := testutil.MakeUuidVectorByString(vs2, nil)
	{
		// test single agg with Grow & Fill function
		agg := agg.NewUnaryDistAgg(true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill)
		err := agg.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		v, err := agg.Eval(m)
		require.NoError(t, err)

		want, err := types.ParseUuid("117a0bd5-2d0d-11ed-940f-000c29847904")
		require.NoError(t, err)

		require.Equal(t, []types.Uuid{want}, vector.GetColumn[types.Uuid](v))
		v.Free(m)
	}
	{
		// test two agg with Merge function
		agg0 := agg.NewUnaryDistAgg(true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill)
		err := agg0.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg0.Fill(0, int64(i), 1, []*vector.Vector{vec})
		}
		agg1 := agg.NewUnaryDistAgg(true, testTyp, testTyp, mn.Grows, mn.Eval, mn.Merge, mn.Fill)
		err = agg1.Grows(1, m)
		require.NoError(t, err)
		for i := 0; i < Rows; i++ {
			agg1.Fill(0, int64(i), 1, []*vector.Vector{vec2})
		}
		agg0.Merge(agg1, 0, 0)
		{
			v, err := agg0.Eval(m)
			require.NoError(t, err)
			want, err := types.ParseUuid("117a0bd5-2d0d-11ed-940f-000c29847904")
			require.NoError(t, err)

			require.Equal(t, []types.Uuid{want}, vector.GetColumn[types.Uuid](v))
			v.Free(m)
		}
		{
			v, err := agg1.Eval(m)
			require.NoError(t, err)
			want, err := types.ParseUuid("149e3f0f-2de4-11ed-940f-000c29847904")
			require.NoError(t, err)

			require.Equal(t, []types.Uuid{want}, vector.GetColumn[types.Uuid](v))
			v.Free(m)
		}
	}
	vec.Free(m)
	vec2.Free(m)
	require.Equal(t, int64(0), m.Size())
}

func makeBytes(values []string) []string {
	return values
}
