// Copyright 2021 Matrix Origin
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

package frontend

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/matrixorigin/matrixone/pkg/config"
	"github.com/matrixorigin/matrixone/pkg/defines"
	mock_frontend "github.com/matrixorigin/matrixone/pkg/frontend/test"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/tree"
	plan2 "github.com/matrixorigin/matrixone/pkg/sql/plan"
	"github.com/matrixorigin/matrixone/pkg/vm/mempool"
	"github.com/matrixorigin/matrixone/pkg/vm/mmu/host"
	"github.com/prashantv/gostub"
	"github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestGetTenantInfo(t *testing.T) {
	convey.Convey("tenant", t, func() {
		type input struct {
			input   string
			output  string
			wantErr bool
		}
		args := []input{
			{"u1", "{tenantInfo sys:u1:moadmin -- 0:0:0}", false},
			{"tenant1:u1", "{tenantInfo tenant1:u1:moadmin -- 0:0:0}", false},
			{"tenant1:u1:r1", "{tenantInfo tenant1:u1:r1 -- 0:0:0}", false},
			{":u1:r1", "{tenantInfo tenant1:u1:r1 -- 0:0:0}", true},
			{"tenant1:u1:", "{tenantInfo tenant1:u1:moadmin -- 0:0:0}", true},
			{"tenant1::r1", "{tenantInfo tenant1::r1 -- 0:0:0}", true},
			{"tenant1:    :r1", "{tenantInfo tenant1::r1 -- 0:0:0}", true},
			{"     : :r1", "{tenantInfo tenant1::r1 -- 0:0:0}", true},
			{"   tenant1   :   u1   :   r1    ", "{tenantInfo tenant1:u1:r1 -- 0:0:0}", false},
		}

		for _, arg := range args {
			ti, err := GetTenantInfo(arg.input)
			if arg.wantErr {
				convey.So(err, convey.ShouldNotBeNil)
			} else {
				convey.So(err, convey.ShouldBeNil)
				tis := ti.String()
				convey.So(tis, convey.ShouldEqual, arg.output)
			}
		}
	})

	convey.Convey("tenant op", t, func() {
		ti := &TenantInfo{}
		convey.So(ti.GetTenant(), convey.ShouldBeEmpty)
		convey.So(ti.GetTenantID(), convey.ShouldBeZeroValue)
		convey.So(ti.GetUser(), convey.ShouldBeEmpty)
		convey.So(ti.GetUserID(), convey.ShouldBeZeroValue)
		convey.So(ti.GetDefaultRole(), convey.ShouldBeEmpty)
		convey.So(ti.GetDefaultRoleID(), convey.ShouldBeZeroValue)

		ti.SetTenantID(10)
		convey.So(ti.GetTenantID(), convey.ShouldEqual, 10)
		ti.SetUserID(10)
		convey.So(ti.GetUserID(), convey.ShouldEqual, 10)
		ti.SetDefaultRoleID(10)
		convey.So(ti.GetDefaultRoleID(), convey.ShouldEqual, 10)

		convey.So(ti.IsSysTenant(), convey.ShouldBeFalse)
		convey.So(ti.IsDefaultRole(), convey.ShouldBeFalse)
		convey.So(ti.IsMoAdminRole(), convey.ShouldBeFalse)

		convey.So(GetDefaultTenant(), convey.ShouldEqual, sysAccountName)
		convey.So(GetDefaultRole(), convey.ShouldEqual, moAdminRoleName)
	})
}

func TestPrivilegeType_Scope(t *testing.T) {
	convey.Convey("scope", t, func() {
		pss := []struct {
			ps PrivilegeScope
			s  string
		}{
			{PrivilegeScopeSys, "sys"},
			{PrivilegeScopeAccount, "account"},
			{PrivilegeScopeUser, "user"},
			{PrivilegeScopeRole, "role"},
			{PrivilegeScopeDatabase, "database"},
			{PrivilegeScopeTable, "table"},
			{PrivilegeScopeRoutine, "routine"},
			{PrivilegeScopeSys | PrivilegeScopeRole | PrivilegeScopeRoutine, "sys,role,routine"},
			{PrivilegeScopeSys | PrivilegeScopeDatabase | PrivilegeScopeTable, "sys,database,table"},
		}
		for _, scope := range pss {
			convey.So(scope.ps.String(), convey.ShouldEqual, scope.s)
		}
	})
}

func TestPrivilegeType(t *testing.T) {
	convey.Convey("privilege type", t, func() {
		type arg struct {
			pt PrivilegeType
			s  string
			sc PrivilegeScope
		}
		args := []arg{}
		for i := PrivilegeTypeCreateAccount; i <= PrivilegeTypeExecute; i++ {
			args = append(args, arg{pt: i, s: i.String(), sc: i.Scope()})
		}
		for _, a := range args {
			convey.So(a.pt.String(), convey.ShouldEqual, a.s)
			convey.So(a.pt.Scope(), convey.ShouldEqual, a.sc)
		}
	})
}

func TestFormSql(t *testing.T) {
	convey.Convey("form sql", t, func() {
		convey.So(getSqlForCheckTenant("a"), convey.ShouldEqual, fmt.Sprintf(checkTenantFormat, "a"))
		convey.So(getSqlForPasswordOfUser("u"), convey.ShouldEqual, fmt.Sprintf(getPasswordOfUserFormat, "u"))
		convey.So(getSqlForCheckRoleExists(0, "r"), convey.ShouldEqual, fmt.Sprintf(checkRoleExistsFormat, 0, "r"))
		convey.So(getSqlForRoleIdOfRole("r"), convey.ShouldEqual, fmt.Sprintf(roleIdOfRoleFormat, "r"))
		convey.So(getSqlForRoleOfUser(0, "r"), convey.ShouldEqual, fmt.Sprintf(getRoleOfUserFormat, 0, "r"))
	})
}

func Test_checkSysExistsOrNot(t *testing.T) {
	convey.Convey("check sys tenant exists or not", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		bh.EXPECT().ClearExecResultSet().Return().AnyTimes()

		mrs1 := mock_frontend.NewMockExecResult(ctrl)
		dbs := make([]string, 0)
		for k := range sysWantedDatabases {
			dbs = append(dbs, k)
		}
		mrs1.EXPECT().GetRowCount().Return(uint64(len(sysWantedDatabases))).AnyTimes()
		mrs1.EXPECT().GetString(gomock.Any(), gomock.Any()).DoAndReturn(func(r uint64, c uint64) (string, error) {
			return dbs[r], nil
		}).AnyTimes()

		mrs2 := mock_frontend.NewMockExecResult(ctrl)
		tables := make([]string, 0)
		for k := range sysWantedTables {
			tables = append(tables, k)
		}

		mrs2.EXPECT().GetRowCount().Return(uint64(len(sysWantedTables))).AnyTimes()
		mrs2.EXPECT().GetString(gomock.Any(), gomock.Any()).DoAndReturn(func(r uint64, c uint64) (string, error) {
			return tables[r], nil
		}).AnyTimes()

		rs := []ExecResult{
			mrs1,
			mrs2,
		}

		cnt := 0
		bh.EXPECT().GetExecResultSet().DoAndReturn(func() []interface{} {
			old := cnt
			cnt++
			if cnt >= len(rs) {
				cnt = 0
			}
			return []interface{}{rs[old]}
		}).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		exists, err := checkSysExistsOrNot(ctx, pu)
		convey.So(exists, convey.ShouldBeTrue)
		convey.So(err, convey.ShouldBeNil)

		err = InitSysTenant(ctx)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_createTablesInMoCatalog(t *testing.T) {
	convey.Convey("createTablesInMoCatalog", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		tenant := &TenantInfo{
			Tenant:        sysAccountName,
			User:          rootName,
			DefaultRole:   moAdminRoleName,
			TenantID:      sysAccountID,
			UserID:        rootID,
			DefaultRoleID: moAdminRoleID,
		}

		err := createTablesInMoCatalog(ctx, tenant, pu)
		convey.So(err, convey.ShouldBeNil)

		err = createTablesInInformationSchema(ctx, tenant, pu)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_checkTenantExistsOrNot(t *testing.T) {
	convey.Convey("check tenant exists or not", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		mrs1 := mock_frontend.NewMockExecResult(ctrl)
		mrs1.EXPECT().GetRowCount().Return(uint64(1)).AnyTimes()

		bh.EXPECT().GetExecResultSet().Return([]interface{}{mrs1}).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		exists, err := checkTenantExistsOrNot(ctx, pu, "test")
		convey.So(exists, convey.ShouldBeTrue)
		convey.So(err, convey.ShouldBeNil)

		tenant := &TenantInfo{
			Tenant:        sysAccountName,
			User:          rootName,
			DefaultRole:   moAdminRoleName,
			TenantID:      sysAccountID,
			UserID:        rootID,
			DefaultRoleID: moAdminRoleID,
		}

		err = InitGeneralTenant(ctx, tenant, &tree.CreateAccount{Name: "test", IfNotExists: true})
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_createTablesInMoCatalogOfGeneralTenant(t *testing.T) {
	convey.Convey("createTablesInMoCatalog", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		tenant := &TenantInfo{
			Tenant:        sysAccountName,
			User:          rootName,
			DefaultRole:   moAdminRoleName,
			TenantID:      sysAccountID,
			UserID:        rootID,
			DefaultRoleID: moAdminRoleID,
		}

		ca := &tree.CreateAccount{
			Name:        "test",
			IfNotExists: true,
			AuthOption: tree.AccountAuthOption{
				AdminName:      "test_root",
				IdentifiedType: tree.AccountIdentified{Typ: tree.AccountIdentifiedByPassword, Str: "123"}},
			Comment: tree.AccountComment{Exist: true, Comment: "test acccount"},
		}

		newTi, err := createTablesInMoCatalogOfGeneralTenant(ctx, tenant, pu, ca)
		convey.So(err, convey.ShouldBeNil)

		err = createTablesInInformationSchemaOfGeneralTenant(ctx, tenant, pu, newTi)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_checkUserExistsOrNot(t *testing.T) {
	convey.Convey("checkUserExistsOrNot", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		mrs1 := mock_frontend.NewMockExecResult(ctrl)
		mrs1.EXPECT().GetRowCount().Return(uint64(1)).AnyTimes()

		bh.EXPECT().GetExecResultSet().Return([]interface{}{mrs1}).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		exists, err := checkUserExistsOrNot(ctx, pu, "test")
		convey.So(exists, convey.ShouldBeTrue)
		convey.So(err, convey.ShouldBeNil)

		tenant := &TenantInfo{
			Tenant:        sysAccountName,
			User:          rootName,
			DefaultRole:   moAdminRoleName,
			TenantID:      sysAccountID,
			UserID:        rootID,
			DefaultRoleID: moAdminRoleID,
		}

		err = InitUser(ctx, tenant, &tree.CreateUser{IfNotExists: true, Users: []*tree.User{{Username: "test"}}})
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_initUser(t *testing.T) {
	convey.Convey("init user", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().ClearExecResultSet().AnyTimes()
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		rs := mock_frontend.NewMockExecResult(ctrl)
		//first time, return 1,
		//second time, return 0,
		cnt := 0
		rs.EXPECT().GetRowCount().DoAndReturn(func() uint64 {
			cnt++
			if cnt == 1 {
				return 1
			} else {
				return 0
			}
		}).AnyTimes()
		rs.EXPECT().GetInt64(gomock.Any(), gomock.Any()).Return(int64(10), nil).AnyTimes()
		bh.EXPECT().GetExecResultSet().Return([]interface{}{rs}).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		tenant := &TenantInfo{
			Tenant:        sysAccountName,
			User:          rootName,
			DefaultRole:   moAdminRoleName,
			TenantID:      sysAccountID,
			UserID:        rootID,
			DefaultRoleID: moAdminRoleID,
		}

		cu := &tree.CreateUser{
			IfNotExists: true,
			Users: []*tree.User{
				{
					Username:   "u1",
					AuthOption: &tree.AccountIdentified{Typ: tree.AccountIdentifiedByPassword, Str: "123"},
				},
				{
					Username:   "u2",
					AuthOption: &tree.AccountIdentified{Typ: tree.AccountIdentifiedByPassword, Str: "123"},
				},
			},
			Role:    &tree.Role{UserName: "test_role"},
			MiscOpt: &tree.UserMiscOptionAccountUnlock{},
		}

		err := InitUser(ctx, tenant, cu)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_initRole(t *testing.T) {
	convey.Convey("init role", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
		pu.SV.SetDefaultValues()

		pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
		pu.Mempool = mempool.New()
		ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

		bh := mock_frontend.NewMockBackgroundExec(ctrl)
		bh.EXPECT().ClearExecResultSet().AnyTimes()
		bh.EXPECT().Close().Return().AnyTimes()
		bh.EXPECT().Exec(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		rs := mock_frontend.NewMockExecResult(ctrl)
		rs.EXPECT().GetRowCount().Return(uint64(0)).AnyTimes()
		bh.EXPECT().GetExecResultSet().Return([]interface{}{rs}).AnyTimes()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		tenant := &TenantInfo{
			Tenant:        sysAccountName,
			User:          rootName,
			DefaultRole:   moAdminRoleName,
			TenantID:      sysAccountID,
			UserID:        rootID,
			DefaultRoleID: moAdminRoleID,
		}

		cr := &tree.CreateRole{
			IfNotExists: true,
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
			},
		}

		err := InitRole(ctx, tenant, cr)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_determinePrivilege(t *testing.T) {
	type arg struct {
		stmt tree.Statement
		priv *privilege
	}

	args := []arg{
		{stmt: &tree.CreateAccount{}},
		{stmt: &tree.DropAccount{}},
		{stmt: &tree.AlterAccount{}},
		{stmt: &tree.CreateUser{}},
		{stmt: &tree.DropUser{}},
		{stmt: &tree.AlterUser{}},
		{stmt: &tree.CreateRole{}},
		{stmt: &tree.DropRole{}},
		{stmt: &tree.GrantRole{}},
		{stmt: &tree.RevokeRole{}},
		{stmt: &tree.GrantPrivilege{}},
		{stmt: &tree.RevokePrivilege{}},
		{stmt: &tree.CreateDatabase{}},
		{stmt: &tree.DropDatabase{}},
		{stmt: &tree.ShowDatabases{}},
		{stmt: &tree.ShowCreateDatabase{}},
		{stmt: &tree.Use{}},
		{stmt: &tree.ShowTables{}},
		{stmt: &tree.ShowCreateTable{}},
		{stmt: &tree.ShowColumns{}},
		{stmt: &tree.ShowCreateView{}},
		{stmt: &tree.CreateTable{}},
		{stmt: &tree.CreateView{}},
		{stmt: &tree.DropTable{}},
		{stmt: &tree.DropView{}},
		{stmt: &tree.Select{}},
		{stmt: &tree.Insert{}},
		{stmt: &tree.Load{}},
		{stmt: &tree.Update{}},
		{stmt: &tree.Delete{}},
		{stmt: &tree.CreateIndex{}},
		{stmt: &tree.DropIndex{}},
		{stmt: &tree.ShowIndex{}},
		{stmt: &tree.ShowProcessList{}},
		{stmt: &tree.ShowErrors{}},
		{stmt: &tree.ShowWarnings{}},
		{stmt: &tree.ShowVariables{}},
		{stmt: &tree.ShowStatus{}},
		{stmt: &tree.ExplainFor{}},
		{stmt: &tree.ExplainAnalyze{}},
		{stmt: &tree.ExplainStmt{}},
		{stmt: &tree.BeginTransaction{}},
		{stmt: &tree.CommitTransaction{}},
		{stmt: &tree.RollbackTransaction{}},
		{stmt: &tree.SetVar{}},
		{stmt: &tree.SetDefaultRole{}},
		{stmt: &tree.SetRole{}},
		{stmt: &tree.SetPassword{}},
		{stmt: &tree.PrepareStmt{}},
		{stmt: &tree.PrepareString{}},
		{stmt: &tree.Deallocate{}},
	}

	for i := 0; i < len(args); i++ {
		args[i].priv = determinePrivilegeSetOfStatement(args[i].stmt)
	}

	convey.Convey("privilege of statement", t, func() {
		for i := 0; i < len(args); i++ {
			priv := determinePrivilegeSetOfStatement(args[i].stmt)
			convey.So(priv, convey.ShouldResemble, args[i].priv)
		}
	})
}

func Test_determineCreateAccount(t *testing.T) {
	convey.Convey("create/drop/alter account succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateAccount{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := [][]interface{}{
			{0, true},
		}

		sql2result := makeSql2ExecResult(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create/drop/alter account fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateAccount{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := [][]interface{}{}

		//actually no role dependency loop
		roleIdsInMoRoleGrant := []int{0, 1}
		rowsOfMoRoleGrant := [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineCreateUser(t *testing.T) {
	convey.Convey("create user succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateUser{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//without privilege create user, all
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{
			{0, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create user succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateUser{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create user, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege create user
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create user fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateUser{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create user, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege create user, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege create user, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineDropUser(t *testing.T) {
	convey.Convey("drop/alter user succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropUser{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege drop user
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		//without privilege all
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter user succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropUser{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop user, all, account/user ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege drop user
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter user fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropUser{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop user, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege drop user, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege drop user, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineCreateRole(t *testing.T) {
	convey.Convey("create role succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege create role
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create role succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create role, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege create role
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create role fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create role, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege create role, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege create role, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineDropRole(t *testing.T) {
	convey.Convey("drop/alter role succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege drop role
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter role succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege drop role
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter role fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineGrantRole(t *testing.T) {
	convey.Convey("grant role succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.GrantRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege manage grants
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})

	convey.Convey("grant role succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.GrantRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege manage grants, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege manage grants
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)
		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})

	convey.Convey("grant role succ 3 (mo_role_grant + with_grant_option)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		roleNames := []string{
			"r1",
			"r2",
			"r3",
		}

		gr := &tree.GrantRole{}
		for _, name := range roleNames {
			gr.Roles = append(gr.Roles, &tree.Role{UserName: name})
		}
		priv := determinePrivilegeSetOfStatement(gr)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 5, 6, 7}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege manage grants, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege manage grants
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		rowsOfMoRolePrivs[3][0] = [][]interface{}{}
		rowsOfMoRolePrivs[3][1] = [][]interface{}{}

		rowsOfMoRolePrivs[4][0] = [][]interface{}{}
		rowsOfMoRolePrivs[4][1] = [][]interface{}{}

		//grant role 1,5,6,7 to role 0
		roleIdsInMoRoleGrant := []int{0, 1, 5, 6, 7}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
			{5, true},
			{6, true},
			{7, true},
		}
		rowsOfMoRoleGrant[1] = [][]interface{}{}
		rowsOfMoRoleGrant[2] = [][]interface{}{}
		rowsOfMoRoleGrant[3] = [][]interface{}{}
		rowsOfMoRoleGrant[4] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		//fill mo_role
		rowsOfMoRole := make([][][]interface{}, len(roleNames))
		rowsOfMoRole[0] = [][]interface{}{
			{5},
		}
		rowsOfMoRole[1] = [][]interface{}{
			{6},
		}
		rowsOfMoRole[2] = [][]interface{}{
			{7},
		}
		makeRowsOfMoRole(sql2result, roleNames, rowsOfMoRole)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, gr)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})

	convey.Convey("grant role succ 4 (mo_user_grant + with_grant_option)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		roleNames := []string{
			"r1",
			"r2",
			"r3",
		}

		gr := &tree.GrantRole{}
		for _, name := range roleNames {
			gr.Roles = append(gr.Roles, &tree.Role{UserName: name})
		}
		priv := determinePrivilegeSetOfStatement(gr)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
			{5, true},
			{6, true},
			{7, true},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 5, 6, 7}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege manage grants, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege manage grants
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		rowsOfMoRolePrivs[3][0] = [][]interface{}{}
		rowsOfMoRolePrivs[3][1] = [][]interface{}{}

		rowsOfMoRolePrivs[4][0] = [][]interface{}{}
		rowsOfMoRolePrivs[4][1] = [][]interface{}{}

		//grant role 1,5,6,7 to role 0
		roleIdsInMoRoleGrant := []int{0, 1, 5, 6, 7}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRoleGrant[1] = [][]interface{}{}
		rowsOfMoRoleGrant[2] = [][]interface{}{}
		rowsOfMoRoleGrant[3] = [][]interface{}{}
		rowsOfMoRoleGrant[4] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		//fill mo_role
		rowsOfMoRole := make([][][]interface{}, len(roleNames))
		rowsOfMoRole[0] = [][]interface{}{
			{5},
		}
		rowsOfMoRole[1] = [][]interface{}{
			{6},
		}
		rowsOfMoRole[2] = [][]interface{}{
			{7},
		}
		makeRowsOfMoRole(sql2result, roleNames, rowsOfMoRole)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, gr)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})

	convey.Convey("grant role fail 1 (mo_user_grant + with_grant_option)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		roleNames := []string{
			"r1",
			"r2",
			"r3",
		}

		gr := &tree.GrantRole{}
		for _, name := range roleNames {
			gr.Roles = append(gr.Roles, &tree.Role{UserName: name})
		}
		priv := determinePrivilegeSetOfStatement(gr)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
			{5, true},
			{6, false},
			{7, true},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 5, 6, 7}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege manage grants, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege manage grants
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		rowsOfMoRolePrivs[3][0] = [][]interface{}{}
		rowsOfMoRolePrivs[3][1] = [][]interface{}{}

		rowsOfMoRolePrivs[4][0] = [][]interface{}{}
		rowsOfMoRolePrivs[4][1] = [][]interface{}{}

		//grant role 1,5,6,7 to role 0
		roleIdsInMoRoleGrant := []int{0, 1, 5, 6, 7}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRoleGrant[1] = [][]interface{}{}
		rowsOfMoRoleGrant[2] = [][]interface{}{}
		rowsOfMoRoleGrant[3] = [][]interface{}{}
		rowsOfMoRoleGrant[4] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		//fill mo_role
		rowsOfMoRole := make([][][]interface{}, len(roleNames))
		rowsOfMoRole[0] = [][]interface{}{
			{5},
		}
		rowsOfMoRole[1] = [][]interface{}{
			{6},
		}
		rowsOfMoRole[2] = [][]interface{}{
			{7},
		}
		makeRowsOfMoRole(sql2result, roleNames, rowsOfMoRole)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, gr)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})

	convey.Convey("grant role fail 2 (mo_role_grant + with_grant_option)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		roleNames := []string{
			"r1",
			"r2",
			"r3",
		}

		gr := &tree.GrantRole{}
		for _, name := range roleNames {
			gr.Roles = append(gr.Roles, &tree.Role{UserName: name})
		}
		priv := determinePrivilegeSetOfStatement(gr)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 5, 6, 7}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege manage grants, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege manage grants
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		rowsOfMoRolePrivs[3][0] = [][]interface{}{}
		rowsOfMoRolePrivs[3][1] = [][]interface{}{}

		rowsOfMoRolePrivs[4][0] = [][]interface{}{}
		rowsOfMoRolePrivs[4][1] = [][]interface{}{}

		//grant role 1,5,6,7 to role 0
		roleIdsInMoRoleGrant := []int{0, 1, 5, 6, 7}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
			{5, true},
			{6, false},
			{7, true},
		}
		rowsOfMoRoleGrant[1] = [][]interface{}{}
		rowsOfMoRoleGrant[2] = [][]interface{}{}
		rowsOfMoRoleGrant[3] = [][]interface{}{}
		rowsOfMoRoleGrant[4] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		//fill mo_role
		rowsOfMoRole := make([][][]interface{}, len(roleNames))
		rowsOfMoRole[0] = [][]interface{}{
			{5},
		}
		rowsOfMoRole[1] = [][]interface{}{
			{6},
		}
		rowsOfMoRole[2] = [][]interface{}{
			{7},
		}
		makeRowsOfMoRole(sql2result, roleNames, rowsOfMoRole)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, gr)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineRevokeRole(t *testing.T) {
	convey.Convey("revoke role succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.RevokeRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege manage grants
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})

	convey.Convey("revoke role succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.RevokeRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege manage grants, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege manage grants
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})

	convey.Convey("revoke role fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.RevokeRole{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege drop role, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineGrantPrivilege(t *testing.T) {
	convey.Convey("convert ast privilege", t, func() {
		type arg struct {
			pt   tree.PrivilegeType
			want PrivilegeType
		}

		args := []arg{
			{tree.PRIVILEGE_TYPE_STATIC_SELECT, PrivilegeTypeSelect},
		}

		for _, a := range args {
			w := convertAstPrivilegeTypeToPrivilegeType(a.pt)
			convey.So(w, convey.ShouldEqual, a.want)
		}
	})
	convey.Convey("grant privilege [ObjectType: Table] AdminRole succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.SetDatabaseName("db")

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}
	})
	convey.Convey("grant privilege [ObjectType: Table] succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_DATABASE_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_DATABASE_TABLE,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_TABLE,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}
			ses.SetDatabaseName("db")
			//TODO: make sql2result
			bh.init()
			for _, p := range stmt.Privileges {
				sql, err := formSqlFromGrantPrivilege(context.TODO(), ses, stmt, p)
				convey.So(err, convey.ShouldBeNil)
				makeRowsOfWithGrantOptionPrivilege(bh.sql2result, sql, [][]interface{}{
					{1, true},
				})
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}
	})
	convey.Convey("grant privilege [ObjectType: Table] fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_DATABASE_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_DATABASE_TABLE,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_TABLE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_TABLE,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}
			ses.SetDatabaseName("db")
			//TODO: make sql2result
			bh.init()
			for i, p := range stmt.Privileges {
				sql, err := formSqlFromGrantPrivilege(context.TODO(), ses, stmt, p)
				convey.So(err, convey.ShouldBeNil)
				var rows [][]interface{}
				if i == 0 {
					rows = [][]interface{}{}
				} else {
					rows = [][]interface{}{
						{1, true},
					}
				}
				makeRowsOfWithGrantOptionPrivilege(bh.sql2result, sql, rows)
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		}
	})
	convey.Convey("grant privilege [ObjectType: Database] succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_DATABASE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_DATABASE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_DATABASE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_DATABASE,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}
			ses.SetDatabaseName("db")
			//TODO: make sql2result
			bh.init()
			for _, p := range stmt.Privileges {
				sql, err := formSqlFromGrantPrivilege(context.TODO(), ses, stmt, p)
				convey.So(err, convey.ShouldBeNil)
				makeRowsOfWithGrantOptionPrivilege(bh.sql2result, sql, [][]interface{}{
					{1, true},
				})
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}
	})
	convey.Convey("grant privilege [ObjectType: Database] fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_DATABASE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_DATABASE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR_STAR,
				},
			},
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_DATABASE,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_DATABASE,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}
			ses.SetDatabaseName("db")
			//TODO: make sql2result
			bh.init()
			for i, p := range stmt.Privileges {
				sql, err := formSqlFromGrantPrivilege(context.TODO(), ses, stmt, p)
				convey.So(err, convey.ShouldBeNil)
				var rows [][]interface{}
				if i == 0 {
					rows = [][]interface{}{}
				} else {
					rows = [][]interface{}{
						{1, true},
					}
				}
				makeRowsOfWithGrantOptionPrivilege(bh.sql2result, sql, rows)
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		}
	})
	convey.Convey("grant privilege [ObjectType: Account] succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_ACCOUNT,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}
			ses.SetDatabaseName("db")
			//TODO: make sql2result
			bh.init()
			for _, p := range stmt.Privileges {
				sql, err := formSqlFromGrantPrivilege(context.TODO(), ses, stmt, p)
				convey.So(err, convey.ShouldBeNil)
				makeRowsOfWithGrantOptionPrivilege(bh.sql2result, sql, [][]interface{}{
					{1, true},
				})
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}
	})
	convey.Convey("grant privilege [ObjectType: Account] fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmts := []*tree.GrantPrivilege{
			{
				Privileges: []*tree.Privilege{
					{Type: tree.PRIVILEGE_TYPE_STATIC_SELECT},
					{Type: tree.PRIVILEGE_TYPE_STATIC_INSERT},
				},
				ObjType: tree.OBJECT_TYPE_ACCOUNT,
				Level: &tree.PrivilegeLevel{
					Level: tree.PRIVILEGE_LEVEL_TYPE_STAR,
				},
			},
		}

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}
			ses.SetDatabaseName("db")
			//TODO: make sql2result
			bh.init()
			for i, p := range stmt.Privileges {
				sql, err := formSqlFromGrantPrivilege(context.TODO(), ses, stmt, p)
				convey.So(err, convey.ShouldBeNil)
				var rows [][]interface{}
				if i == 0 {
					rows = [][]interface{}{}
				} else {
					rows = [][]interface{}{
						{1, true},
					}
				}
				makeRowsOfWithGrantOptionPrivilege(bh.sql2result, sql, rows)
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		}
	})
}

func Test_determineRevokePrivilege(t *testing.T) {
	convey.Convey("revoke privilege [ObjectType: Table] AdminRole succ", t, func() {
		var stmts []*tree.RevokePrivilege

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}
	})
	convey.Convey("revoke privilege [ObjectType: Table] not AdminRole fail", t, func() {
		var stmts []*tree.RevokePrivilege

		for _, stmt := range stmts {
			priv := determinePrivilegeSetOfStatement(stmt)
			ses := newSes(priv)
			ses.tenant = &TenantInfo{
				Tenant:        "xxx",
				User:          "xxx",
				DefaultRole:   "xxx",
				TenantID:      1001,
				UserID:        1001,
				DefaultRoleID: 1001,
			}

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeNone(ses.GetRequestContext(), ses, stmt)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		}
	})
}

func Test_determineCreateDatabase(t *testing.T) {
	convey.Convey("create database succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateDatabase{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//without privilege create database, all
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{
			{0, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create database succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateDatabase{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create database, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege create database
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create database fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateDatabase{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create database, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege create database, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege create database, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineDropDatabase(t *testing.T) {
	convey.Convey("drop/alter database succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropDatabase{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege drop database
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		//without privilege all
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter database succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropDatabase{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop database, all, account/user ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege drop database
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)
		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter database fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropDatabase{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop database, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege drop database, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege drop database, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)
		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineShowDatabase(t *testing.T) {
	convey.Convey("show database succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.ShowDatabases{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege show databases
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		//without privilege all
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("show database succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.ShowDatabases{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege show databases, all, account/user ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege show databases
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("show database fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.ShowDatabases{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege show databases, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege show databases, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege show databases, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineUseDatabase(t *testing.T) {
	convey.Convey("use database succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.Use{
			Name: "db",
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege show databases
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		//without privilege all
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("use database succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.Use{
			Name: "db",
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege show databases, all, account/user ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege show databases
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("use database fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.Use{
			Name: "db",
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege show databases, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege show databases, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege show databases, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineUseRole(t *testing.T) {
	//TODO:add ut
}

func Test_determineCreateTable(t *testing.T) {
	convey.Convey("create table succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateTable{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//without privilege create table, all
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{
			{0, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create table succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateTable{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create table, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege create table
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("create table fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.CreateTable{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege create table, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege create table, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege create table, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineDropTable(t *testing.T) {
	convey.Convey("drop/alter table succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropTable{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//with privilege drop table
		rowsOfMoRolePrivs[0][0] = [][]interface{}{
			{0, true},
		}
		//without privilege all
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			nil, nil)

		bh := newBh(ctrl, sql2result)
		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter table succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropTable{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop table, all, account/user ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 with privilege drop table
		rowsOfMoRolePrivs[1][0] = [][]interface{}{
			{1, true},
		}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//grant role 1 to role 0
		roleIdsInMoRoleGrant := []int{0}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)
		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeTrue)
	})
	convey.Convey("drop/alter table fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		stmt := &tree.DropTable{}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		rowsOfMoUserGrant := [][]interface{}{
			{0, false},
		}
		roleIdsInMoRolePrivs := []int{0, 1, 2}
		rowsOfMoRolePrivs := make([][][][]interface{}, len(roleIdsInMoRolePrivs))
		for i := 0; i < len(roleIdsInMoRolePrivs); i++ {
			rowsOfMoRolePrivs[i] = make([][][]interface{}, len(priv.entries))
		}

		//role 0 without privilege drop table, all, ownership
		rowsOfMoRolePrivs[0][0] = [][]interface{}{}
		rowsOfMoRolePrivs[0][1] = [][]interface{}{}

		//role 1 without privilege drop table, all, ownership
		rowsOfMoRolePrivs[1][0] = [][]interface{}{}
		rowsOfMoRolePrivs[1][1] = [][]interface{}{}

		//role 2 without privilege drop table, all, ownership
		rowsOfMoRolePrivs[2][0] = [][]interface{}{}
		rowsOfMoRolePrivs[2][1] = [][]interface{}{}

		roleIdsInMoRoleGrant := []int{0, 1, 2}
		rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
		//grant role 1 to role 0
		rowsOfMoRoleGrant[0] = [][]interface{}{
			{1, true},
		}
		//grant role 2 to role 1
		rowsOfMoRoleGrant[1] = [][]interface{}{
			{2, true},
		}
		rowsOfMoRoleGrant[2] = [][]interface{}{}

		sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
			roleIdsInMoRolePrivs, priv.entries, rowsOfMoRolePrivs,
			roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

		bh := newBh(ctrl, sql2result)

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		ok, err := authenticatePrivilegeOfStatementWithObjectTypeAccountAndDatabase(ses.GetRequestContext(), ses, nil)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ok, convey.ShouldBeFalse)
	})
}

func Test_determineDML(t *testing.T) {
	type arg struct {
		stmt tree.Statement
		p    *plan2.Plan
	}

	args := []arg{
		{
			stmt: &tree.Select{},
			p: &plan2.Plan{
				Plan: &plan2.Plan_Query{
					Query: &plan2.Query{
						Nodes: []*plan2.Node{
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "t", ObjName: "a"}},
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "s", ObjName: "b"}},
						},
					},
				},
			},
		},
		{
			stmt: &tree.Update{},
			p: &plan2.Plan{
				Plan: &plan2.Plan_Query{
					Query: &plan2.Query{
						Nodes: []*plan2.Node{
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "t", ObjName: "a"}},
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "s", ObjName: "b"}},
							{NodeType: plan.Node_UPDATE},
						},
					},
				},
			},
		},
		{
			stmt: &tree.Delete{},
			p: &plan2.Plan{
				Plan: &plan2.Plan_Query{
					Query: &plan2.Query{
						Nodes: []*plan2.Node{
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "t", ObjName: "a"}},
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "s", ObjName: "b"}},
							{NodeType: plan.Node_DELETE},
						},
					},
				},
			},
		},
		{ //insert into values
			stmt: &tree.Insert{},
			p: &plan2.Plan{
				Plan: &plan.Plan_Ins{
					Ins: &plan.InsertValues{
						DbName:  "t",
						TblName: "a",
					},
				},
			},
		},
		{ //insert into select
			stmt: &tree.Insert{},
			p: &plan2.Plan{
				Plan: &plan2.Plan_Query{
					Query: &plan2.Query{
						Nodes: []*plan2.Node{
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "t", ObjName: "a"}},
							{NodeType: plan.Node_TABLE_SCAN, ObjRef: &plan2.ObjectRef{SchemaName: "s", ObjName: "b"}},
							{NodeType: plan.Node_INSERT, ObjRef: &plan2.ObjectRef{SchemaName: "s", ObjName: "b"}},
						},
					},
				},
			},
		},
	}

	convey.Convey("select/update/delete/insert succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		for _, a := range args {
			priv := determinePrivilegeSetOfStatement(a.stmt)
			ses := newSes(priv)

			rowsOfMoUserGrant := [][]interface{}{
				{0, false},
			}

			sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
				nil, nil, nil,
				nil, nil)

			arr := extractPrivilegeTipsFromPlan(a.p)
			convertPrivilegeTipsToPrivilege(priv, arr)

			roleIds := []int{
				int(ses.tenant.GetDefaultRoleID()),
			}

			for _, roleId := range roleIds {
				for _, entry := range priv.entries {
					sql := getSqlForCheckRoleHasTableLevelPrivilege(int64(roleId), entry.privilegeId, entry.databaseName, entry.tableName)
					sql2result[sql] = newMrsForWithGrantOptionPrivilege([][]interface{}{
						{entry.privilegeId, true},
					})
				}
			}

			bh := newBh(ctrl, sql2result)
			bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
			defer bhStub.Reset()

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeTable(ses.GetRequestContext(), ses, a.stmt, a.p)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}

	})

	convey.Convey("select/update/delete/insert succ 2", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		for _, a := range args {
			priv := determinePrivilegeSetOfStatement(a.stmt)
			ses := newSes(priv)

			rowsOfMoUserGrant := [][]interface{}{
				{0, false},
			}

			//grant role 1 to role 0
			roleIdsInMoRoleGrant := []int{0}
			rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
			rowsOfMoRoleGrant[0] = [][]interface{}{
				{1, true},
			}

			sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
				nil, nil, nil,
				roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

			arr := extractPrivilegeTipsFromPlan(a.p)
			convertPrivilegeTipsToPrivilege(priv, arr)

			//role 0 does not have the select
			//role 1 has the select
			roleIds := []int{
				int(ses.tenant.GetDefaultRoleID()), 1,
			}

			for _, roleId := range roleIds {
				for _, entry := range priv.entries {
					sql, _ := getSqlFromPrivilegeEntry(int64(roleId), entry)
					var rows [][]interface{}
					if roleId == 1 {
						rows = [][]interface{}{
							{entry.privilegeId, true},
						}
					}
					sql2result[sql] = newMrsForWithGrantOptionPrivilege(rows)
				}
			}

			bh := newBh(ctrl, sql2result)

			bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
			defer bhStub.Reset()

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeTable(ses.GetRequestContext(), ses, a.stmt, a.p)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeTrue)
		}
	})

	convey.Convey("select/update/delete/insert fail", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		for _, a := range args {
			priv := determinePrivilegeSetOfStatement(a.stmt)
			ses := newSes(priv)

			rowsOfMoUserGrant := [][]interface{}{
				{0, false},
			}

			//grant role 1 to role 0
			roleIdsInMoRoleGrant := []int{0, 1}
			rowsOfMoRoleGrant := make([][][]interface{}, len(roleIdsInMoRoleGrant))
			rowsOfMoRoleGrant[0] = [][]interface{}{
				{1, true},
			}
			rowsOfMoRoleGrant[0] = [][]interface{}{}

			sql2result := makeSql2ExecResult2(0, rowsOfMoUserGrant,
				nil, nil, nil,
				roleIdsInMoRoleGrant, rowsOfMoRoleGrant)

			arr := extractPrivilegeTipsFromPlan(a.p)
			convertPrivilegeTipsToPrivilege(priv, arr)

			//role 0,1 does not have the select
			roleIds := []int{
				int(ses.tenant.GetDefaultRoleID()), 1,
			}

			for _, roleId := range roleIds {
				for _, entry := range priv.entries {
					sql, _ := getSqlFromPrivilegeEntry(int64(roleId), entry)
					rows := make([][]interface{}, 0)
					sql2result[sql] = newMrsForWithGrantOptionPrivilege(rows)
				}
			}

			bh := newBh(ctrl, sql2result)

			bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
			defer bhStub.Reset()

			ok, err := authenticatePrivilegeOfStatementWithObjectTypeTable(ses.GetRequestContext(), ses, a.stmt, a.p)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ok, convey.ShouldBeFalse)
		}
	})
}

func Test_doGrantRole(t *testing.T) {
	convey.Convey("grant role to role succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "r4"},
				{Username: "r5"},
				{Username: "r6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i + len(stmt.Roles)},
			})

			bh.sql2result[sql] = mrs
		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckRoleGrant([][]interface{}{})
				bh.sql2result[sql] = mrs
			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("grant role to user succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u4"},
				{Username: "u5"},
				{Username: "u6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to empty roles,
		//init to users
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{})

			bh.sql2result[sql] = mrs

			sql = getSqlForPasswordOfUser(user.Username)
			mrs = newMrsForPasswordOfUser([][]interface{}{
				{i, "111", i},
			})
			bh.sql2result[sql] = mrs
		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckRoleGrant([][]interface{}{})
				bh.sql2result[sql] = mrs

				sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckUserGrant([][]interface{}{})
				bh.sql2result[sql] = mrs
			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("grant role to role+user succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u4"},
				{Username: "u5"},
				{Username: "u6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to 2 roles,
		//init to 1 users
		for i, user := range stmt.Users {
			if i < 2 { //roles
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{
					{i + len(stmt.Roles)},
				})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{})
				bh.sql2result[sql] = mrs
			} else { //users
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{
					{i, "111", i},
				})
				bh.sql2result[sql] = mrs
			}

		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				if toId < 2 { //roles
					toId = toId + len(stmt.Roles)
					sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckRoleGrant([][]interface{}{})
					bh.sql2result[sql] = mrs

					sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckUserGrant([][]interface{}{})
					bh.sql2result[sql] = mrs
				} else { //users
					sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckRoleGrant([][]interface{}{})
					bh.sql2result[sql] = mrs

					sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckUserGrant([][]interface{}{})
					bh.sql2result[sql] = mrs
				}

			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("grant role to role+user 2 (insert) succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u4"},
				{Username: "u5"},
				{Username: "u6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to 2 roles,
		//init to 1 users
		for i, user := range stmt.Users {
			if i < 2 { //roles
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{
					{i + len(stmt.Roles)},
				})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{})
				bh.sql2result[sql] = mrs
			} else { //users
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{
					{i, "111", i},
				})
				bh.sql2result[sql] = mrs
			}

		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{
			{0, 1, true},
			{1, 2, true},
			{3, 4, true},
		})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				if toId < 2 { //roles
					toId = toId + len(stmt.Roles)
					sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckRoleGrant([][]interface{}{})
					bh.sql2result[sql] = mrs

					sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckUserGrant([][]interface{}{})
					bh.sql2result[sql] = mrs
				} else { //users
					sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckRoleGrant([][]interface{}{})
					bh.sql2result[sql] = mrs

					sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckUserGrant([][]interface{}{})
					bh.sql2result[sql] = mrs
				}
			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("grant role to role+user 3 (update) succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u4"},
				{Username: "u5"},
				{Username: "u6"},
			},
			GrantOption: true,
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to 2 roles,
		//init to 1 users
		for i, user := range stmt.Users {
			if i < 2 { //roles
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{
					{i + len(stmt.Roles)},
				})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{})
				bh.sql2result[sql] = mrs
			} else { //users
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{
					{i, "111", i},
				})
				bh.sql2result[sql] = mrs
			}

		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{
			{0, 1, true},
			{1, 2, true},
			{3, 4, true},
		})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				if toId < 2 { //roles
					toId = toId + len(stmt.Roles)
					sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckRoleGrant([][]interface{}{
						{fromId, toId, false},
					})
					bh.sql2result[sql] = mrs

					//sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					//mrs = newMrsForCheckUserGrant([][]interface{}{})
					//bh.sql2result[sql] = mrs
				} else { //users
					//sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					//mrs = newMrsForCheckRoleGrant([][]interface{}{})
					//bh.sql2result[sql] = mrs

					sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckUserGrant([][]interface{}{
						{fromId, toId, false},
					})
					bh.sql2result[sql] = mrs
				}

			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("grant role to role fail direct loop", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
			},
			Users: []*tree.User{
				{Username: "r1"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i + len(stmt.Roles)},
			})

			bh.sql2result[sql] = mrs
		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckRoleGrant([][]interface{}{})
				bh.sql2result[sql] = mrs
			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeError)
	})
	convey.Convey("grant role to role+user fail indirect loop", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "r4"},
				{Username: "r5"},
				{Username: "u6"},
			},
			GrantOption: true,
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to 2 roles,
		//init to 1 users
		for i, user := range stmt.Users {
			if i < 2 { //roles
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{
					{i + len(stmt.Roles)},
				})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{})
				bh.sql2result[sql] = mrs
			} else { //users
				sql := getSqlForRoleIdOfRole(user.Username)
				mrs := newMrsForRoleIdOfRole([][]interface{}{})

				bh.sql2result[sql] = mrs

				sql = getSqlForPasswordOfUser(user.Username)
				mrs = newMrsForPasswordOfUser([][]interface{}{
					{i, "111", i},
				})
				bh.sql2result[sql] = mrs
			}

		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{
			{1, 0, true},
			{2, 1, true},
			{3, 2, true},
			{4, 2, true},
		})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				if toId < 2 { //roles
					toId = toId + len(stmt.Roles)
					sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckRoleGrant([][]interface{}{
						{fromId, toId, false},
					})
					bh.sql2result[sql] = mrs

					//sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					//mrs = newMrsForCheckUserGrant([][]interface{}{})
					//bh.sql2result[sql] = mrs
				} else { //users
					//sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
					//mrs = newMrsForCheckRoleGrant([][]interface{}{})
					//bh.sql2result[sql] = mrs

					sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
					mrs = newMrsForCheckUserGrant([][]interface{}{
						{fromId, toId, false},
					})
					bh.sql2result[sql] = mrs
				}

			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeError)
	})
	convey.Convey("grant role to role fail no role", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "r4"},
				{Username: "r5"},
				{Username: "r6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for _, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{})
			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i + len(stmt.Roles)},
			})

			bh.sql2result[sql] = mrs
		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckRoleGrant([][]interface{}{})
				bh.sql2result[sql] = mrs
			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeError)
	})
	convey.Convey("grant role to user fail no user", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u4"},
				{Username: "u5"},
				{Username: "u6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to empty roles,
		//init to users
		for _, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{})

			bh.sql2result[sql] = mrs

			sql = getSqlForPasswordOfUser(user.Username)
			mrs = newMrsForPasswordOfUser([][]interface{}{})
			bh.sql2result[sql] = mrs
		}

		//has "ro roles", need init mo_role_grant (assume empty)
		sql := getSqlForGetAllStuffRoleGrantFormat()
		mrs := newMrsForGetAllStuffRoleGrant([][]interface{}{})

		bh.sql2result[sql] = mrs

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				sql = getSqlForCheckRoleGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckRoleGrant([][]interface{}{})
				bh.sql2result[sql] = mrs

				sql = getSqlForCheckUserGrant(int64(fromId), int64(toId))
				mrs = newMrsForCheckUserGrant([][]interface{}{})
				bh.sql2result[sql] = mrs
			}
		}

		err := doGrantRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeError)
	})
}

func Test_doRevokeRole(t *testing.T) {
	convey.Convey("revoke role from role succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.RevokeRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "r4"},
				{Username: "r5"},
				{Username: "r6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i + len(stmt.Roles)},
			})

			bh.sql2result[sql] = mrs
		}

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql := getSqlForDeleteRoleGrant(int64(fromId), int64(toId))
				bh.sql2result[sql] = nil
			}
		}

		err := doRevokeRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("revoke role from role succ (if exists = true, miss role before FROM)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.RevokeRole{
			IfExists: true,
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "r4"},
				{Username: "r5"},
				{Username: "r6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		var mrs *MysqlResultSet
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			if i == 0 {
				mrs = newMrsForRoleIdOfRole([][]interface{}{})
			} else {
				mrs = newMrsForRoleIdOfRole([][]interface{}{
					{i},
				})
			}
			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i + len(stmt.Roles)},
			})

			bh.sql2result[sql] = mrs
		}

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql := getSqlForDeleteRoleGrant(int64(fromId), int64(toId))
				bh.sql2result[sql] = nil
			}
		}

		err := doRevokeRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
	convey.Convey("revoke role from role fail (if exists = false,miss role before FROM)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.RevokeRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "r4"},
				{Username: "r5"},
				{Username: "r6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		var mrs *MysqlResultSet
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			if i == 0 {
				mrs = newMrsForRoleIdOfRole([][]interface{}{})
			} else {
				mrs = newMrsForRoleIdOfRole([][]interface{}{
					{i},
				})
			}

			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i + len(stmt.Roles)},
			})

			bh.sql2result[sql] = mrs
		}

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql := getSqlForDeleteRoleGrant(int64(fromId), int64(toId))
				bh.sql2result[sql] = nil
			}
		}

		err := doRevokeRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeError)
	})
	convey.Convey("revoke role from user fail (if exists = false,miss role after FROM)", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.RevokeRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u1"},
				{Username: "u2"},
				{Username: "u3"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		var mrs *MysqlResultSet
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs = newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})

			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			//sql := getSqlForRoleIdOfRole(user.Username)
			//mrs = newMrsForRoleIdOfRole([][]interface{}{})

			sql := getSqlForPasswordOfUser(user.Username)
			//miss u2
			if i == 1 {
				mrs = newMrsForPasswordOfUser([][]interface{}{})
			} else {
				mrs = newMrsForPasswordOfUser([][]interface{}{
					{i + len(stmt.Roles)},
				})
			}

			bh.sql2result[sql] = mrs
		}

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql := getSqlForDeleteUserGrant(int64(fromId), int64(toId))
				bh.sql2result[sql] = nil
			}
		}

		err := doRevokeRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeError)
	})
	convey.Convey("revoke role from user succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.RevokeRole{
			Roles: []*tree.Role{
				{UserName: "r1"},
				{UserName: "r2"},
				{UserName: "r3"},
			},
			Users: []*tree.User{
				{Username: "u4"},
				{Username: "u5"},
				{Username: "u6"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		//init to roles
		for i, user := range stmt.Users {
			sql := getSqlForRoleIdOfRole(user.Username)
			mrs := newMrsForRoleIdOfRole([][]interface{}{})

			bh.sql2result[sql] = mrs

			sql = getSqlForPasswordOfUser(user.Username)
			mrs = newMrsForPasswordOfUser([][]interface{}{
				{i},
			})

			bh.sql2result[sql] = mrs
		}

		//loop on from ... to
		for fromId := range stmt.Roles {
			for toId := range stmt.Users {
				toId = toId + len(stmt.Roles)
				sql := getSqlForDeleteRoleGrant(int64(fromId), int64(toId))
				bh.sql2result[sql] = nil
			}
		}

		err := doRevokeRole(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
}

func Test_doGrantPrivilege(t *testing.T) {
	convey.Convey("grant object type account to user succ", t, func() {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		bh := &backgroundExecTest{}
		bh.init()

		bhStub := gostub.StubFunc(&NewBackgroundHandler, bh)
		defer bhStub.Reset()

		stmt := &tree.GrantPrivilege{
			Privileges: []*tree.Privilege{
				{Type: tree.PRIVILEGE_TYPE_STATIC_CREATE_DATABASE},
			},
			ObjType: tree.OBJECT_TYPE_ACCOUNT,
			Level:   &tree.PrivilegeLevel{Level: tree.PRIVILEGE_LEVEL_TYPE_STAR},
			Roles: []*tree.Role{
				{UserName: "r1"},
			},
		}
		priv := determinePrivilegeSetOfStatement(stmt)
		ses := newSes(priv)

		//no result set
		bh.sql2result["begin;"] = nil
		bh.sql2result["commit;"] = nil
		bh.sql2result["rollback;"] = nil

		//init from roles
		for i, role := range stmt.Roles {
			sql := getSqlForRoleIdOfRole(role.UserName)
			mrs := newMrsForRoleIdOfRole([][]interface{}{
				{i},
			})
			bh.sql2result[sql] = mrs
		}

		for _, p := range stmt.Privileges {
			privType := convertAstPrivilegeTypeToPrivilegeType(p.Type)
			for j := range stmt.Roles {
				sql := getSqlForCheckRoleHasPrivilege(int64(j), objectTypeAccount, objectIDAll, int64(privType))
				mrs := newMrsForCheckRoleHasPrivilege([][]interface{}{})
				bh.sql2result[sql] = mrs
			}
		}

		err := doGrantPrivilege(ses.GetRequestContext(), ses, stmt)
		convey.So(err, convey.ShouldBeNil)
	})
}

func newSes(priv *privilege) *Session {
	pu := config.NewParameterUnit(&config.FrontendParameters{}, nil, nil, nil, nil, nil)
	pu.SV.SetDefaultValues()

	pu.HostMmu = host.New(pu.SV.HostMmuLimitation)
	pu.Mempool = mempool.New()
	ctx := context.WithValue(context.TODO(), config.ParameterUnitKey, pu)

	proto := NewMysqlClientProtocol(0, nil, 1024, pu.SV)

	ses := NewSession(proto, nil, nil, pu, gSysVariables)
	tenant := &TenantInfo{
		Tenant:        sysAccountName,
		User:          rootName,
		DefaultRole:   moAdminRoleName,
		TenantID:      sysAccountID,
		UserID:        rootID,
		DefaultRoleID: moAdminRoleID,
	}
	ses.SetTenantInfo(tenant)
	ses.priv = priv
	ses.SetRequestContext(ctx)
	return ses
}

func newBh(ctrl *gomock.Controller, sql2result map[string]ExecResult) BackgroundExec {
	var currentSql string
	bh := mock_frontend.NewMockBackgroundExec(ctrl)
	bh.EXPECT().ClearExecResultSet().AnyTimes()
	bh.EXPECT().Close().Return().AnyTimes()
	bh.EXPECT().Exec(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, sql string) error {
		currentSql = sql
		return nil
	}).AnyTimes()
	bh.EXPECT().GetExecResultSet().DoAndReturn(func() []interface{} {
		return []interface{}{sql2result[currentSql]}
	}).AnyTimes()
	return bh
}

type backgroundExecTest struct {
	currentSql string
	sql2result map[string]ExecResult
}

func (bt *backgroundExecTest) init() {
	bt.sql2result = make(map[string]ExecResult)
}

func (bt *backgroundExecTest) Close() {
}

func (bt *backgroundExecTest) Exec(ctx context.Context, s string) error {
	bt.currentSql = s
	return nil
}

func (bt *backgroundExecTest) GetExecResultSet() []interface{} {
	return []interface{}{bt.sql2result[bt.currentSql]}
}

func (bt *backgroundExecTest) ClearExecResultSet() {
	//bt.init()
}

var _ BackgroundExec = &backgroundExecTest{}

func newMrsForRoleIdOfRole(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("role_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	mrs.AddColumn(col1)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForRoleIdOfUserId(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("role_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("with_grant_option")
	col2.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForCheckRoleHasPrivilege(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("role_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("with_grant_option")
	col2.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForInheritedRoleIdOfRoleId(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("granted_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("with_grant_option")
	col2.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForGetAllStuffRoleGrant(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("granted_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("grantee_id")
	col2.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col3 := &MysqlColumn{}
	col3.SetName("with_grant_option")
	col3.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)
	mrs.AddColumn(col3)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForCheckRoleGrant(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("granted_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("grantee_id")
	col2.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col3 := &MysqlColumn{}
	col3.SetName("with_grant_option")
	col3.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)
	mrs.AddColumn(col3)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForPasswordOfUser(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("user_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("authentication_string")
	col2.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col3 := &MysqlColumn{}
	col3.SetName("default_role")
	col3.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)
	mrs.AddColumn(col3)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func newMrsForCheckUserGrant(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("role_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("user_id")
	col2.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col3 := &MysqlColumn{}
	col3.SetName("with_grant_option")
	col3.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)
	mrs.AddColumn(col3)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func makeRowsOfMoRole(sql2result map[string]ExecResult, roleNames []string, rows [][][]interface{}) {
	for i, name := range roleNames {
		sql2result[getSqlForRoleIdOfRole(name)] = newMrsForRoleIdOfRole(rows[i])
	}
}

func makeRowsOfMoUserGrant(sql2result map[string]ExecResult, userId int, rows [][]interface{}) {
	sql2result[getSqlForRoleIdOfUserId(userId)] = newMrsForRoleIdOfUserId(rows)
}

func makeRowsOfMoRolePrivs(sql2result map[string]ExecResult, roleIds []int, entries []privilegeEntry, rowsOfMoRolePrivs [][]interface{}) {
	for _, roleId := range roleIds {
		for _, entry := range entries {
			sql, _ := getSqlFromPrivilegeEntry(int64(roleId), entry)
			sql2result[sql] = newMrsForCheckRoleHasPrivilege(rowsOfMoRolePrivs)
		}
	}
}

func makeRowsOfMoRoleGrant(sql2result map[string]ExecResult, roleIds []int, rowsOfMoRoleGrant [][]interface{}) {
	for _, roleId := range roleIds {
		sql := getSqlForInheritedRoleIdOfRoleId(int64(roleId))
		sql2result[sql] = newMrsForInheritedRoleIdOfRoleId(rowsOfMoRoleGrant)
	}
}

func newMrsForWithGrantOptionPrivilege(rows [][]interface{}) *MysqlResultSet {
	mrs := &MysqlResultSet{}

	col1 := &MysqlColumn{}
	col1.SetName("privilege_id")
	col1.SetColumnType(defines.MYSQL_TYPE_LONGLONG)

	col2 := &MysqlColumn{}
	col2.SetName("with_grant_option")
	col2.SetColumnType(defines.MYSQL_TYPE_BOOL)

	mrs.AddColumn(col1)
	mrs.AddColumn(col2)

	for _, row := range rows {
		mrs.AddRow(row)
	}

	return mrs
}

func makeRowsOfWithGrantOptionPrivilege(sql2result map[string]ExecResult, sql string, rows [][]interface{}) {
	sql2result[sql] = newMrsForWithGrantOptionPrivilege(rows)
}

func makeSql2ExecResult(userId int,
	rowsOfMoUserGrant [][]interface{},
	roleIdsInMoRolePrivs []int, entries []privilegeEntry, rowsOfMoRolePrivs [][]interface{},
	roleIdsInMoRoleGrant []int, rowsOfMoRoleGrant [][]interface{}) map[string]ExecResult {
	sql2result := make(map[string]ExecResult)
	makeRowsOfMoUserGrant(sql2result, userId, rowsOfMoUserGrant)
	makeRowsOfMoRolePrivs(sql2result, roleIdsInMoRolePrivs, entries, rowsOfMoRolePrivs)
	makeRowsOfMoRoleGrant(sql2result, roleIdsInMoRoleGrant, rowsOfMoRoleGrant)
	return sql2result
}

func makeSql2ExecResult2(userId int,
	rowsOfMoUserGrant [][]interface{},
	roleIdsInMoRolePrivs []int, entries []privilegeEntry, rowsOfMoRolePrivs [][][][]interface{},
	roleIdsInMoRoleGrant []int, rowsOfMoRoleGrant [][][]interface{}) map[string]ExecResult {
	sql2result := make(map[string]ExecResult)
	makeRowsOfMoUserGrant(sql2result, userId, rowsOfMoUserGrant)
	for i, roleId := range roleIdsInMoRolePrivs {
		for j, entry := range entries {
			sql, _ := getSqlFromPrivilegeEntry(int64(roleId), entry)
			sql2result[sql] = newMrsForCheckRoleHasPrivilege(rowsOfMoRolePrivs[i][j])
		}
	}

	for i, roleId := range roleIdsInMoRoleGrant {
		sql := getSqlForInheritedRoleIdOfRoleId(int64(roleId))
		sql2result[sql] = newMrsForInheritedRoleIdOfRoleId(rowsOfMoRoleGrant[i])
	}
	return sql2result
}

func Test_graph(t *testing.T) {
	convey.Convey("create graph", t, func() {
		g := NewGraph()

		g.addEdge(1, 2)
		g.addEdge(2, 3)
		g.addEdge(3, 4)

		convey.So(g.hasLoop(1), convey.ShouldBeFalse)

		g2 := NewGraph()
		g2.addEdge(1, 2)
		g2.addEdge(2, 3)
		g2.addEdge(3, 4)
		e1 := g2.addEdge(4, 1)

		convey.So(g2.hasLoop(1), convey.ShouldBeTrue)

		g2.removeEdge(e1)
		convey.So(g2.hasLoop(1), convey.ShouldBeFalse)

		g2.addEdge(4, 1)
		convey.So(g2.hasLoop(1), convey.ShouldBeTrue)
	})
}
