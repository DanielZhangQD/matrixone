// Copyright 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plan

import (
	"encoding/json"

	"github.com/matrixorigin/matrixone/pkg/common/moerr"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/pb/plan"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql"
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/tree"
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog"
)

func NewQueryBuilder(queryType plan.Query_StatementType, ctx CompilerContext) *QueryBuilder {
	return &QueryBuilder{
		qry: &Query{
			StmtType: queryType,
		},
		compCtx:      ctx,
		ctxByNode:    []*BindContext{},
		nameByColRef: make(map[[2]int32]string),
		nextTag:      0,
	}
}

func (builder *QueryBuilder) remapExpr(expr *Expr, colMap map[[2]int32][2]int32) error {
	switch ne := expr.Expr.(type) {
	case *plan.Expr_Col:
		mapId := [2]int32{ne.Col.RelPos, ne.Col.ColPos}
		if ids, ok := colMap[mapId]; ok {
			ne.Col.RelPos = ids[0]
			ne.Col.ColPos = ids[1]
			ne.Col.Name = builder.nameByColRef[mapId]
		} else {
			return moerr.NewParseError("can't find column in context's map %v", colMap)
		}

	case *plan.Expr_F:
		for _, arg := range ne.F.GetArgs() {
			err := builder.remapExpr(arg, colMap)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type ColRefRemapping struct {
	globalToLocal map[[2]int32][2]int32
	localToGlobal [][2]int32
}

func (m *ColRefRemapping) addColRef(colRef [2]int32) {
	m.globalToLocal[colRef] = [2]int32{0, int32(len(m.localToGlobal))}
	m.localToGlobal = append(m.localToGlobal, colRef)
}

func (builder *QueryBuilder) remapAllColRefs(nodeID int32, colRefCnt map[[2]int32]int) (*ColRefRemapping, error) {
	node := builder.qry.Nodes[nodeID]

	remapping := &ColRefRemapping{
		globalToLocal: make(map[[2]int32][2]int32),
	}

	switch node.NodeType {
	case plan.Node_TABLE_SCAN, plan.Node_MATERIAL_SCAN, plan.Node_EXTERNAL_SCAN:
		for _, expr := range node.FilterList {
			increaseRefCnt(expr, colRefCnt)
		}

		internalRemapping := &ColRefRemapping{
			globalToLocal: make(map[[2]int32][2]int32),
		}

		tag := node.BindingTags[0]
		newTableDef := &plan.TableDef{
			Name:          node.TableDef.Name,
			Defs:          node.TableDef.Defs,
			Name2ColIndex: node.TableDef.Name2ColIndex,
			Createsql:     node.TableDef.Createsql,
		}

		for i, col := range node.TableDef.Cols {
			globalRef := [2]int32{tag, int32(i)}
			if colRefCnt[globalRef] == 0 {
				continue
			}

			internalRemapping.addColRef(globalRef)

			newTableDef.Cols = append(newTableDef.Cols, col)
		}

		if len(newTableDef.Cols) == 0 {
			internalRemapping.addColRef([2]int32{tag, 0})
			newTableDef.Cols = append(newTableDef.Cols, node.TableDef.Cols[0])
		}

		node.TableDef = newTableDef

		for _, expr := range node.FilterList {
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, internalRemapping.globalToLocal)
			if err != nil {
				return nil, err
			}
		}

		for i, col := range node.TableDef.Cols {
			if colRefCnt[internalRemapping.localToGlobal[i]] == 0 {
				continue
			}

			remapping.addColRef(internalRemapping.localToGlobal[i])

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: col.Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: int32(i),
						Name:   builder.nameByColRef[internalRemapping.localToGlobal[i]],
					},
				},
			})
		}

		if len(node.ProjectList) == 0 {
			globalRef := [2]int32{tag, 0}
			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: node.TableDef.Cols[0].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: 0,
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

	case plan.Node_INTERSECT, plan.Node_INTERSECT_ALL,
		plan.Node_UNION, plan.Node_UNION_ALL,
		plan.Node_MINUS, plan.Node_MINUS_ALL:

		thisTag := node.BindingTags[0]
		leftID := node.Children[0]
		rightID := node.Children[1]
		for i, expr := range node.ProjectList {
			increaseRefCnt(expr, colRefCnt)
			globalRef := [2]int32{thisTag, int32(i)}
			remapping.addColRef(globalRef)
		}

		rightNode := builder.qry.Nodes[rightID]
		if rightNode.NodeType == plan.Node_PROJECT {
			projectTag := rightNode.BindingTags[0]
			for i := range rightNode.ProjectList {
				increaseRefCnt(&plan.Expr{
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: projectTag,
							ColPos: int32(i),
						},
					}}, colRefCnt)
			}
		}

		internalMap := make(map[[2]int32][2]int32)

		leftRemapping, err := builder.remapAllColRefs(leftID, colRefCnt)
		if err != nil {
			return nil, err
		}
		for k, v := range leftRemapping.globalToLocal {
			internalMap[k] = v
		}

		_, err = builder.remapAllColRefs(rightID, colRefCnt)
		if err != nil {
			return nil, err
		}

		for _, expr := range node.ProjectList {
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, internalMap)
			if err != nil {
				return nil, err
			}
		}

	case plan.Node_JOIN:
		for _, expr := range node.OnList {
			increaseRefCnt(expr, colRefCnt)
		}

		internalMap := make(map[[2]int32][2]int32)

		leftID := node.Children[0]
		leftRemapping, err := builder.remapAllColRefs(leftID, colRefCnt)
		if err != nil {
			return nil, err
		}

		for k, v := range leftRemapping.globalToLocal {
			internalMap[k] = v
		}

		rightID := node.Children[1]
		rightRemapping, err := builder.remapAllColRefs(rightID, colRefCnt)
		if err != nil {
			return nil, err
		}

		for k, v := range rightRemapping.globalToLocal {
			internalMap[k] = [2]int32{1, v[1]}
		}

		for _, expr := range node.OnList {
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, internalMap)
			if err != nil {
				return nil, err
			}
		}

		childProjList := builder.qry.Nodes[leftID].ProjectList
		for i, globalRef := range leftRemapping.localToGlobal {
			if colRefCnt[globalRef] == 0 {
				continue
			}

			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: childProjList[i].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: int32(i),
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

		if node.JoinType == plan.Node_MARK {
			globalRef := [2]int32{node.BindingTags[0], 0}
			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: &plan.Type{
					Id:       int32(types.T_bool),
					Nullable: true,
					Size:     1,
				},
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: -1,
						ColPos: 0,
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})

			break
		}

		if node.JoinType != plan.Node_SEMI && node.JoinType != plan.Node_ANTI {
			childProjList = builder.qry.Nodes[rightID].ProjectList
			for i, globalRef := range rightRemapping.localToGlobal {
				if colRefCnt[globalRef] == 0 {
					continue
				}

				remapping.addColRef(globalRef)

				node.ProjectList = append(node.ProjectList, &plan.Expr{
					Typ: childProjList[i].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: 1,
							ColPos: int32(i),
							Name:   builder.nameByColRef[globalRef],
						},
					},
				})
			}
		}

		if len(node.ProjectList) == 0 && len(leftRemapping.localToGlobal) > 0 {
			globalRef := leftRemapping.localToGlobal[0]
			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: builder.qry.Nodes[leftID].ProjectList[0].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: 0,
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

	case plan.Node_AGG:
		for _, expr := range node.GroupBy {
			increaseRefCnt(expr, colRefCnt)
		}

		for _, expr := range node.AggList {
			increaseRefCnt(expr, colRefCnt)
		}

		childRemapping, err := builder.remapAllColRefs(node.Children[0], colRefCnt)
		if err != nil {
			return nil, err
		}

		groupTag := node.BindingTags[0]
		aggregateTag := node.BindingTags[1]

		for idx, expr := range node.GroupBy {
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, childRemapping.globalToLocal)
			if err != nil {
				return nil, err
			}

			globalRef := [2]int32{groupTag, int32(idx)}
			if colRefCnt[globalRef] == 0 {
				continue
			}

			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: expr.Typ,
				Expr: &plan.Expr_Col{
					Col: &ColRef{
						RelPos: -1,
						ColPos: int32(idx),
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

		for idx, expr := range node.AggList {
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, childRemapping.globalToLocal)
			if err != nil {
				return nil, err
			}

			globalRef := [2]int32{aggregateTag, int32(idx)}
			if colRefCnt[globalRef] == 0 {
				continue
			}

			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &Expr{
				Typ: expr.Typ,
				Expr: &plan.Expr_Col{
					Col: &ColRef{
						RelPos: -2,
						ColPos: int32(idx),
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

		if len(node.ProjectList) == 0 {
			if len(node.GroupBy) > 0 {
				globalRef := [2]int32{groupTag, 0}
				remapping.addColRef(globalRef)

				node.ProjectList = append(node.ProjectList, &plan.Expr{
					Typ: node.GroupBy[0].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: -1,
							ColPos: 0,
							Name:   builder.nameByColRef[globalRef],
						},
					},
				})
			} else {
				globalRef := [2]int32{aggregateTag, 0}
				remapping.addColRef(globalRef)

				node.ProjectList = append(node.ProjectList, &plan.Expr{
					Typ: node.AggList[0].Typ,
					Expr: &plan.Expr_Col{
						Col: &plan.ColRef{
							RelPos: -2,
							ColPos: 0,
							Name:   builder.nameByColRef[globalRef],
						},
					},
				})
			}
		}

	case plan.Node_SORT:
		for _, orderBy := range node.OrderBy {
			increaseRefCnt(orderBy.Expr, colRefCnt)
		}

		childRemapping, err := builder.remapAllColRefs(node.Children[0], colRefCnt)
		if err != nil {
			return nil, err
		}

		for _, orderBy := range node.OrderBy {
			decreaseRefCnt(orderBy.Expr, colRefCnt)
			err := builder.remapExpr(orderBy.Expr, childRemapping.globalToLocal)
			if err != nil {
				return nil, err
			}
		}

		childProjList := builder.qry.Nodes[node.Children[0]].ProjectList
		for i, globalRef := range childRemapping.localToGlobal {
			if colRefCnt[globalRef] == 0 {
				continue
			}

			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: childProjList[i].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: int32(i),
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

		if len(node.ProjectList) == 0 && len(childRemapping.localToGlobal) > 0 {
			globalRef := childRemapping.localToGlobal[0]
			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: childProjList[0].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: 0,
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

	case plan.Node_FILTER:
		for _, expr := range node.FilterList {
			increaseRefCnt(expr, colRefCnt)
		}

		childRemapping, err := builder.remapAllColRefs(node.Children[0], colRefCnt)
		if err != nil {
			return nil, err
		}

		for _, expr := range node.FilterList {
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, childRemapping.globalToLocal)
			if err != nil {
				return nil, err
			}
		}

		childProjList := builder.qry.Nodes[node.Children[0]].ProjectList
		for i, globalRef := range childRemapping.localToGlobal {
			if colRefCnt[globalRef] == 0 {
				continue
			}

			remapping.addColRef(globalRef)

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: childProjList[i].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: int32(i),
						Name:   builder.nameByColRef[globalRef],
					},
				},
			})
		}

		if len(node.ProjectList) == 0 {
			if len(childRemapping.localToGlobal) > 0 {
				remapping.addColRef(childRemapping.localToGlobal[0])
			}

			node.ProjectList = append(node.ProjectList, &plan.Expr{
				Typ: childProjList[0].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: 0,
					},
				},
			})
		}

	case plan.Node_PROJECT, plan.Node_MATERIAL:
		projectTag := node.BindingTags[0]

		var neededProj []int32

		for i, expr := range node.ProjectList {
			globalRef := [2]int32{projectTag, int32(i)}
			if colRefCnt[globalRef] == 0 {
				continue
			}

			neededProj = append(neededProj, int32(i))
			increaseRefCnt(expr, colRefCnt)
		}

		if len(neededProj) == 0 {
			increaseRefCnt(node.ProjectList[0], colRefCnt)
			neededProj = append(neededProj, 0)
		}

		childRemapping, err := builder.remapAllColRefs(node.Children[0], colRefCnt)
		if err != nil {
			return nil, err
		}

		var newProjList []*plan.Expr
		for _, needed := range neededProj {
			expr := node.ProjectList[needed]
			decreaseRefCnt(expr, colRefCnt)
			err := builder.remapExpr(expr, childRemapping.globalToLocal)
			if err != nil {
				return nil, err
			}

			globalRef := [2]int32{projectTag, needed}
			remapping.addColRef(globalRef)

			newProjList = append(newProjList, expr)
		}

		node.ProjectList = newProjList

	case plan.Node_DISTINCT:
		childRemapping, err := builder.remapAllColRefs(node.Children[0], colRefCnt)
		if err != nil {
			return nil, err
		}

		// Rewrite DISTINCT to AGG
		node.NodeType = plan.Node_AGG
		preNode := builder.qry.Nodes[node.Children[0]]
		node.GroupBy = make([]*Expr, len(preNode.ProjectList))
		node.ProjectList = make([]*Expr, len(preNode.ProjectList))

		for i, prjExpr := range preNode.ProjectList {
			node.GroupBy[i] = &plan.Expr{
				Typ: prjExpr.Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: 0,
						ColPos: int32(i),
					},
				},
			}

			node.ProjectList[i] = &plan.Expr{
				Typ: prjExpr.Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: -1,
						ColPos: int32(i),
					},
				},
			}
		}

		remapping = childRemapping

	case plan.Node_VALUE_SCAN:
		// VALUE_SCAN always have one column now
		node.ProjectList = append(node.ProjectList, &plan.Expr{
			Typ:  &plan.Type{Id: int32(types.T_int64)},
			Expr: &plan.Expr_C{C: &plan.Const{Value: &plan.Const_Ival{Ival: 0}}},
		})

	default:
		return nil, moerr.NewInternalError("unsupport node type")
	}

	node.BindingTags = nil

	return remapping, nil
}

func (builder *QueryBuilder) createQuery() (*Query, error) {
	for i, rootId := range builder.qry.Steps {
		rootId, _ = builder.pushdownFilters(rootId, nil)
		rootId = builder.determineJoinOrder(rootId)
		rootId = builder.pushdownSemiAntiJoins(rootId)
		builder.qry.Steps[i] = rootId

		colRefCnt := make(map[[2]int32]int)
		rootNode := builder.qry.Nodes[rootId]
		resultTag := rootNode.BindingTags[0]
		for i := range rootNode.ProjectList {
			colRefCnt[[2]int32{resultTag, int32(i)}] = 1
		}

		_, err := builder.remapAllColRefs(rootId, colRefCnt)
		if err != nil {
			return nil, err
		}
	}
	return builder.qry, nil
}

func (builder *QueryBuilder) buildUnion(stmt *tree.UnionClause, astOrderBy tree.OrderBy, astLimit *tree.Limit, ctx *BindContext, isRoot bool) (int32, error) {
	var selectStmts []tree.Statement
	var unionTypes []plan.Node_NodeType

	// get Union selectStmts
	err := getUnionSelects(stmt, &selectStmts, &unionTypes)
	if err != nil {
		return 0, err
	}

	if len(selectStmts) == 1 {
		switch sltStmt := selectStmts[0].(type) {
		case *tree.Select:
			if sltClause, ok := sltStmt.Select.(*tree.SelectClause); ok {
				sltClause.Distinct = true
				return builder.buildSelect(sltStmt, ctx, isRoot)
			} else {
				// rewrite sltStmt to select distinct * from (sltStmt) a
				tmpSltStmt := &tree.Select{
					Select: &tree.SelectClause{
						Distinct: true,

						Exprs: []tree.SelectExpr{
							{Expr: tree.StarExpr()},
						},
						From: &tree.From{
							Tables: tree.TableExprs{
								&tree.AliasedTableExpr{
									Expr: &tree.ParenTableExpr{
										Expr: sltStmt,
									},
									As: tree.AliasClause{
										Alias: "a",
									},
								},
							},
						},
					},
					Limit:   astLimit,
					OrderBy: astOrderBy,
				}
				return builder.buildSelect(tmpSltStmt, ctx, isRoot)
			}

		case *tree.SelectClause:
			if !sltStmt.Distinct {
				sltStmt.Distinct = true
			}
			return builder.buildSelect(&tree.Select{Select: sltStmt, Limit: astLimit, OrderBy: astOrderBy}, ctx, isRoot)
		}
	}

	// build selects
	var projectTypList [][]types.Type
	selectStmtLength := len(selectStmts)
	nodes := make([]int32, selectStmtLength)
	subCtxList := make([]*BindContext, selectStmtLength)
	var projectLength int
	var nodeID int32
	for idx, sltStmt := range selectStmts {
		subCtx := NewBindContext(builder, ctx)
		if slt, ok := sltStmt.(*tree.Select); ok {
			nodeID, err = builder.buildSelect(slt, subCtx, isRoot)
		} else {
			nodeID, err = builder.buildSelect(&tree.Select{Select: sltStmt}, subCtx, isRoot)
		}
		if err != nil {
			return 0, err
		}

		if idx == 0 {
			projectLength = len(builder.qry.Nodes[nodeID].ProjectList)
			projectTypList = make([][]types.Type, projectLength)
			for i := 0; i < projectLength; i++ {
				projectTypList[i] = make([]types.Type, selectStmtLength)
			}
		} else {
			if projectLength != len(builder.qry.Nodes[nodeID].ProjectList) {
				return 0, moerr.NewParseError("SELECT statements have different number of columns")
			}
		}

		for i, expr := range subCtx.results {
			projectTypList[i][idx] = makeTypeByPlan2Expr(expr)
		}
		subCtxList[idx] = subCtx
		nodes[idx] = nodeID
	}

	// reset all select's return Projection(type cast up)
	// we use coalesce function's type check&type cast rule
	for columnIdx, argsType := range projectTypList {
		// we don't cast null as any type in function
		// but we will cast null as some target type in union/intersect/minus
		var tmpArgsType []types.Type
		for _, typ := range argsType {
			if typ.Oid != types.T_any {
				tmpArgsType = append(tmpArgsType, typ)
			}
		}

		if len(tmpArgsType) > 0 {
			_, _, argsCastType, err := function.GetFunctionByName("coalesce", tmpArgsType)
			if err != nil {
				return 0, moerr.NewParseError("the %d column cann't cast to a same type", columnIdx)
			}
			var targetType *plan.Type
			var targetArgType types.Type
			if len(argsCastType) == 0 {
				targetType = makePlan2Type(&tmpArgsType[0])
				targetArgType = tmpArgsType[0]
			} else {
				targetType = makePlan2Type(&argsCastType[0])
				targetArgType = argsCastType[0]
			}

			for idx, tmpID := range nodes {
				if !argsType[idx].Eq(targetArgType) {
					node := builder.qry.Nodes[tmpID]
					if argsType[idx].Oid == types.T_any {
						node.ProjectList[columnIdx].Typ = targetType
					} else {
						node.ProjectList[columnIdx], err = appendCastBeforeExpr(node.ProjectList[columnIdx], targetType)
						if err != nil {
							return 0, err
						}
					}
				}
			}
		}
	}

	firstSelectProjectNode := builder.qry.Nodes[nodes[0]]

	getProjectList := func(tag int32) []*plan.Expr {
		projectList := make([]*plan.Expr, len(firstSelectProjectNode.ProjectList))
		for i, expr := range firstSelectProjectNode.ProjectList {
			projectList[i] = &plan.Expr{
				Typ: expr.Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: tag,
						ColPos: int32(i),
					},
				},
			}
		}
		return projectList
	}

	// build intersect node first.  because intersect has higher precedence then UNION and MINUS
	var newNodes []int32
	var newUnionType []plan.Node_NodeType
	var lastTag int32
	newNodes = append(newNodes, nodes[0])
	for i := 1; i < len(nodes); i++ {
		utIdx := i - 1
		lastNewNodeIdx := len(newNodes) - 1
		if unionTypes[utIdx] == plan.Node_INTERSECT || unionTypes[utIdx] == plan.Node_INTERSECT_ALL {
			lastTag = builder.genNewTag()
			leftNodeTag := builder.qry.Nodes[newNodes[lastNewNodeIdx]].BindingTags[0]
			newNodeID := builder.appendNode(&plan.Node{
				NodeType:    unionTypes[utIdx],
				Children:    []int32{newNodes[lastNewNodeIdx], nodes[i]},
				BindingTags: []int32{lastTag},
				ProjectList: getProjectList(leftNodeTag),
			}, ctx)
			newNodes[lastNewNodeIdx] = newNodeID
		} else {
			newNodes = append(newNodes, nodes[i])
			newUnionType = append(newUnionType, unionTypes[utIdx])
		}
	}

	// build UNION/MINUS node one by one
	lastNodeId := newNodes[0]
	for i := 1; i < len(newNodes); i++ {
		utIdx := i - 1
		lastTag = builder.genNewTag()
		leftNodeTag := builder.qry.Nodes[lastNodeId].BindingTags[0]

		lastNodeId = builder.appendNode(&plan.Node{
			NodeType:    newUnionType[utIdx],
			Children:    []int32{lastNodeId, newNodes[i]},
			BindingTags: []int32{lastTag},
			ProjectList: getProjectList(leftNodeTag),
		}, ctx)
	}

	// set ctx base on selects[0] and it's ctx
	ctx.groupTag = builder.genNewTag()
	ctx.aggregateTag = builder.genNewTag()
	ctx.projectTag = builder.genNewTag()
	// set ctx's headings  projects  results
	ctx.headings = append(ctx.headings, subCtxList[0].headings...)
	for i, v := range ctx.headings {
		ctx.aliasMap[v] = int32(i)
		builder.nameByColRef[[2]int32{lastTag, int32(i)}] = v
		builder.nameByColRef[[2]int32{ctx.projectTag, int32(i)}] = v
	}
	for i, expr := range firstSelectProjectNode.ProjectList {
		ctx.projects = append(ctx.projects, &plan.Expr{
			Typ: expr.Typ,
			Expr: &plan.Expr_Col{
				Col: &plan.ColRef{
					RelPos: lastTag,
					ColPos: int32(i),
				},
			},
		})
	}
	havingBinder := NewHavingBinder(builder, ctx)
	projectionBinder := NewProjectionBinder(builder, ctx, havingBinder)

	// append a project node
	lastNodeId = builder.appendNode(&plan.Node{
		NodeType:    plan.Node_PROJECT,
		ProjectList: ctx.projects,
		Children:    []int32{lastNodeId},
		BindingTags: []int32{ctx.projectTag},
	}, ctx)

	// append orderBy
	if astOrderBy != nil {
		orderBinder := NewOrderBinder(projectionBinder, nil)
		orderBys := make([]*plan.OrderBySpec, 0, len(astOrderBy))

		for _, order := range astOrderBy {
			expr, err := orderBinder.BindExpr(order.Expr)
			if err != nil {
				return 0, err
			}

			orderBy := &plan.OrderBySpec{
				Expr: expr,
			}

			switch order.Direction {
			case tree.DefaultDirection:
				orderBy.Flag = plan.OrderBySpec_INTERNAL
			case tree.Ascending:
				orderBy.Flag = plan.OrderBySpec_ASC
			case tree.Descending:
				orderBy.Flag = plan.OrderBySpec_DESC
			}

			orderBys = append(orderBys, orderBy)
		}

		lastNodeId = builder.appendNode(&plan.Node{
			NodeType: plan.Node_SORT,
			Children: []int32{lastNodeId},
			OrderBy:  orderBys,
		}, ctx)
	}

	// append limit
	if astLimit != nil {
		node := builder.qry.Nodes[lastNodeId]

		limitBinder := NewLimitBinder()
		if astLimit.Offset != nil {
			node.Offset, err = limitBinder.BindExpr(astLimit.Offset, 0, true)
			if err != nil {
				return 0, err
			}
		}
		if astLimit.Count != nil {
			node.Limit, err = limitBinder.BindExpr(astLimit.Count, 0, true)
			if err != nil {
				return 0, err
			}
		}
	}

	// append result PROJECT node
	if builder.qry.Nodes[lastNodeId].NodeType != plan.Node_PROJECT {
		for i := 0; i < len(ctx.projects); i++ {
			ctx.results = append(ctx.results, &plan.Expr{
				Typ: ctx.projects[i].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: ctx.projectTag,
						ColPos: int32(i),
					},
				},
			})
		}
		ctx.resultTag = builder.genNewTag()

		lastNodeId = builder.appendNode(&plan.Node{
			NodeType:    plan.Node_PROJECT,
			ProjectList: ctx.results,
			Children:    []int32{lastNodeId},
			BindingTags: []int32{ctx.resultTag},
		}, ctx)
	} else {
		ctx.results = ctx.projects
	}

	// set heading
	if isRoot {
		builder.qry.Headings = append(builder.qry.Headings, ctx.headings...)
	}

	return lastNodeId, nil
}

func (builder *QueryBuilder) buildSelect(stmt *tree.Select, ctx *BindContext, isRoot bool) (int32, error) {
	// preprocess CTEs
	if stmt.With != nil {
		ctx.cteByName = make(map[string]*CTERef)
		maskedNames := make([]string, len(stmt.With.CTEs))

		for i := range stmt.With.CTEs {
			idx := len(stmt.With.CTEs) - i - 1
			cte := stmt.With.CTEs[idx]

			name := string(cte.Name.Alias)
			if _, ok := ctx.cteByName[name]; ok {
				return 0, moerr.NewSyntaxError("WITH query name %q specified more than once", name)
			}

			var maskedCTEs map[string]any
			if len(maskedNames) > 0 {
				maskedCTEs = make(map[string]any)
				for _, mask := range maskedNames {
					maskedCTEs[mask] = nil
				}
			}

			maskedNames = append(maskedNames, name)

			ctx.cteByName[name] = &CTERef{
				ast:        cte,
				maskedCTEs: maskedCTEs,
			}
		}
	}

	var clause *tree.SelectClause
	astOrderBy := stmt.OrderBy
	astLimit := stmt.Limit

	// strip parentheses
	// ((select a from t1)) order by b  [ is equal ] select a from t1 order by b
	// (((select a from t1)) order by b) [ is equal ] select a from t1 order by b
	//
	// (select a from t1 union/union all select aa from t2) order by a
	//       => we will strip parentheses, but order by only can use 'a' column from the union's output projectlist
	for {
		if selectClause, ok := stmt.Select.(*tree.ParenSelect); ok {
			if selectClause.Select.OrderBy != nil {
				if astOrderBy != nil {
					return 0, moerr.NewSyntaxError("multiple ORDER BY clauses not allowed")
				}
				astOrderBy = selectClause.Select.OrderBy
			}
			if selectClause.Select.Limit != nil {
				if astLimit != nil {
					return 0, moerr.NewSyntaxError("multiple LIMIT clauses not allowed")
				}
				astLimit = selectClause.Select.Limit
			}
			stmt = selectClause.Select
		} else {
			break
		}
	}

	switch selectClause := stmt.Select.(type) {
	case *tree.SelectClause:
		clause = selectClause
	case *tree.UnionClause:
		return builder.buildUnion(selectClause, astOrderBy, astLimit, ctx, isRoot)
	case *tree.ValuesClause:
		return 0, moerr.NewNYI("'SELECT FROM VALUES'")
	default:
		return 0, moerr.NewNYI("statement '%s'", tree.String(stmt, dialect.MYSQL))
	}

	// build FROM clause
	nodeID, err := builder.buildFrom(clause.From.Tables, ctx)
	if err != nil {
		return 0, err
	}

	ctx.binder = NewWhereBinder(builder, ctx)

	// unfold stars and generate headings
	var selectList tree.SelectExprs
	for _, selectExpr := range clause.Exprs {
		switch expr := selectExpr.Expr.(type) {
		case tree.UnqualifiedStar:
			cols, names, err := ctx.unfoldStar("")
			if err != nil {
				return 0, err
			}
			selectList = append(selectList, cols...)
			ctx.headings = append(ctx.headings, names...)

		case *tree.UnresolvedName:
			if expr.Star {
				cols, names, err := ctx.unfoldStar(expr.Parts[0])
				if err != nil {
					return 0, err
				}
				selectList = append(selectList, cols...)
				ctx.headings = append(ctx.headings, names...)
			} else {
				if len(selectExpr.As) > 0 {
					ctx.headings = append(ctx.headings, string(selectExpr.As))
				} else {
					ctx.headings = append(ctx.headings, expr.Parts[0])
				}

				newExpr, err := ctx.qualifyColumnNames(expr, nil, false)
				if err != nil {
					return 0, err
				}

				selectList = append(selectList, tree.SelectExpr{
					Expr: newExpr,
					As:   selectExpr.As,
				})
			}

		default:
			if len(selectExpr.As) > 0 {
				ctx.headings = append(ctx.headings, string(selectExpr.As))
			} else {
				for {
					if parenExpr, ok := expr.(*tree.ParenExpr); ok {
						expr = parenExpr.Expr
					} else {
						break
					}
				}
				ctx.headings = append(ctx.headings, tree.String(expr, dialect.MYSQL))
			}

			newExpr, err := ctx.qualifyColumnNames(expr, nil, false)
			if err != nil {
				return 0, err
			}

			selectList = append(selectList, tree.SelectExpr{
				Expr: newExpr,
				As:   selectExpr.As,
			})
		}
	}

	if len(selectList) == 0 {
		return 0, moerr.NewParseError("No tables used")
	}

	// rewrite right join to left join
	builder.rewriteRightJoinToLeftJoin(nodeID)

	if clause.Where != nil {
		whereList, err := splitAndBindCondition(clause.Where.Expr, ctx)
		if err != nil {
			return 0, err
		}

		var newFilterList []*plan.Expr
		var expr *plan.Expr

		for _, cond := range whereList {
			nodeID, expr, err = builder.flattenSubqueries(nodeID, cond, ctx)
			if err != nil {
				return 0, err
			}

			if expr != nil {
				newFilterList = append(newFilterList, expr)
			}
		}

		nodeID = builder.appendNode(&plan.Node{
			NodeType:   plan.Node_FILTER,
			Children:   []int32{nodeID},
			FilterList: newFilterList,
		}, ctx)
	}

	ctx.groupTag = builder.genNewTag()
	ctx.aggregateTag = builder.genNewTag()
	ctx.projectTag = builder.genNewTag()

	// bind GROUP BY clause
	if clause.GroupBy != nil {
		groupBinder := NewGroupBinder(builder, ctx)
		for _, group := range clause.GroupBy {
			group, err = ctx.qualifyColumnNames(group, nil, false)
			if err != nil {
				return 0, err
			}

			_, err = groupBinder.BindExpr(group, 0, true)
			if err != nil {
				return 0, err
			}
		}
	}

	// bind HAVING clause
	var havingList []*plan.Expr
	havingBinder := NewHavingBinder(builder, ctx)
	if clause.Having != nil {
		ctx.binder = havingBinder
		havingList, err = splitAndBindCondition(clause.Having.Expr, ctx)
		if err != nil {
			return 0, err
		}
	}

	// bind SELECT clause (Projection List)
	projectionBinder := NewProjectionBinder(builder, ctx, havingBinder)
	ctx.binder = projectionBinder
	for i, selectExpr := range selectList {
		astExpr, err := ctx.qualifyColumnNames(selectExpr.Expr, nil, false)
		if err != nil {
			return 0, err
		}

		expr, err := projectionBinder.BindExpr(astExpr, 0, true)
		if err != nil {
			return 0, err
		}

		builder.nameByColRef[[2]int32{ctx.projectTag, int32(i)}] = tree.String(astExpr, dialect.MYSQL)

		alias := string(selectExpr.As)
		if len(alias) > 0 {
			ctx.aliasMap[alias] = int32(len(ctx.projects))
		}
		ctx.projects = append(ctx.projects, expr)
	}

	resultLen := len(ctx.projects)
	for i, proj := range ctx.projects {
		exprStr := proj.String()
		if _, ok := ctx.projectByExpr[exprStr]; !ok {
			ctx.projectByExpr[exprStr] = int32(i)
		}
	}

	ctx.isDistinct = clause.Distinct

	// bind ORDER BY clause
	var orderBys []*plan.OrderBySpec
	if astOrderBy != nil {
		orderBinder := NewOrderBinder(projectionBinder, selectList)
		orderBys = make([]*plan.OrderBySpec, 0, len(astOrderBy))

		for _, order := range astOrderBy {
			expr, err := orderBinder.BindExpr(order.Expr)
			if err != nil {
				return 0, err
			}

			orderBy := &plan.OrderBySpec{
				Expr: expr,
			}

			switch order.Direction {
			case tree.DefaultDirection:
				orderBy.Flag = plan.OrderBySpec_INTERNAL
			case tree.Ascending:
				orderBy.Flag = plan.OrderBySpec_ASC
			case tree.Descending:
				orderBy.Flag = plan.OrderBySpec_DESC
			}

			orderBys = append(orderBys, orderBy)
		}
	}

	// bind limit/offset clause
	var limitExpr *Expr
	var offsetExpr *Expr
	if astLimit != nil {
		limitBinder := NewLimitBinder()
		if astLimit.Offset != nil {
			offsetExpr, err = limitBinder.BindExpr(astLimit.Offset, 0, true)
			if err != nil {
				return 0, err
			}
		}
		if astLimit.Count != nil {
			limitExpr, err = limitBinder.BindExpr(astLimit.Count, 0, true)
			if err != nil {
				return 0, err
			}

			if cExpr, ok := limitExpr.Expr.(*plan.Expr_C); ok {
				if c, ok := cExpr.C.Value.(*plan.Const_Ival); ok {
					ctx.hasSingleRow = c.Ival == 1
				}
			}
		}
	}

	if (len(ctx.groups) > 0 || len(ctx.aggregates) > 0) && len(projectionBinder.boundCols) > 0 {
		return 0, moerr.NewSyntaxError("column %q must appear in the GROUP BY clause or be used in an aggregate function", projectionBinder.boundCols[0])
	}

	// FIXME: delete this when SINGLE join is ready
	if len(ctx.groups) == 0 && len(ctx.aggregates) > 0 {
		ctx.hasSingleRow = true
	}

	// append AGG node
	if len(ctx.groups) > 0 || len(ctx.aggregates) > 0 {
		nodeID = builder.appendNode(&plan.Node{
			NodeType:    plan.Node_AGG,
			Children:    []int32{nodeID},
			GroupBy:     ctx.groups,
			AggList:     ctx.aggregates,
			BindingTags: []int32{ctx.groupTag, ctx.aggregateTag},
		}, ctx)

		if len(havingList) > 0 {
			var newFilterList []*plan.Expr
			var expr *plan.Expr

			for _, cond := range havingList {
				nodeID, expr, err = builder.flattenSubqueries(nodeID, cond, ctx)
				if err != nil {
					return 0, err
				}

				if expr != nil {
					newFilterList = append(newFilterList, expr)
				}
			}

			nodeID = builder.appendNode(&plan.Node{
				NodeType:   plan.Node_FILTER,
				Children:   []int32{nodeID},
				FilterList: newFilterList,
			}, ctx)
		}

		for name, id := range ctx.groupByAst {
			builder.nameByColRef[[2]int32{ctx.groupTag, id}] = name
		}

		for name, id := range ctx.aggregateByAst {
			builder.nameByColRef[[2]int32{ctx.aggregateTag, id}] = name
		}
	}

	// append PROJECT node
	for i, proj := range ctx.projects {
		nodeID, proj, err = builder.flattenSubqueries(nodeID, proj, ctx)
		if err != nil {
			return 0, err
		}

		if proj == nil {
			// TODO: implement MARK join to better support non-scalar subqueries
			return 0, moerr.NewNYI("non-scalar subquery in SELECT clause")
		}

		ctx.projects[i] = proj
	}

	nodeID = builder.appendNode(&plan.Node{
		NodeType:    plan.Node_PROJECT,
		ProjectList: ctx.projects,
		Children:    []int32{nodeID},
		BindingTags: []int32{ctx.projectTag},
	}, ctx)

	// append DISTINCT node
	if clause.Distinct {
		nodeID = builder.appendNode(&plan.Node{
			NodeType: plan.Node_DISTINCT,
			Children: []int32{nodeID},
		}, ctx)
	}

	// append SORT node (include limit, offset)
	if len(orderBys) > 0 {
		nodeID = builder.appendNode(&plan.Node{
			NodeType: plan.Node_SORT,
			Children: []int32{nodeID},
			OrderBy:  orderBys,
		}, ctx)
	}

	if limitExpr != nil || offsetExpr != nil {
		node := builder.qry.Nodes[nodeID]

		node.Limit = limitExpr
		node.Offset = offsetExpr
	}

	// append result PROJECT node
	if builder.qry.Nodes[nodeID].NodeType != plan.Node_PROJECT {
		for i := 0; i < resultLen; i++ {
			ctx.results = append(ctx.results, &plan.Expr{
				Typ: ctx.projects[i].Typ,
				Expr: &plan.Expr_Col{
					Col: &plan.ColRef{
						RelPos: ctx.projectTag,
						ColPos: int32(i),
					},
				},
			})
		}

		ctx.resultTag = builder.genNewTag()

		nodeID = builder.appendNode(&plan.Node{
			NodeType:    plan.Node_PROJECT,
			ProjectList: ctx.results,
			Children:    []int32{nodeID},
			BindingTags: []int32{ctx.resultTag},
		}, ctx)
	} else {
		ctx.results = ctx.projects
	}

	if isRoot {
		builder.qry.Headings = append(builder.qry.Headings, ctx.headings...)
	}

	return nodeID, nil
}

func (builder *QueryBuilder) appendNode(node *plan.Node, ctx *BindContext) int32 {
	nodeID := int32(len(builder.qry.Nodes))
	node.NodeId = nodeID
	builder.qry.Nodes = append(builder.qry.Nodes, node)
	builder.ctxByNode = append(builder.ctxByNode, ctx)

	// TODO: better estimation
	switch node.NodeType {
	case plan.Node_JOIN:
		leftCost := builder.qry.Nodes[node.Children[0]].Cost
		rightCost := builder.qry.Nodes[node.Children[1]].Cost

		switch node.JoinType {
		case plan.Node_INNER:
			card := leftCost.Card * rightCost.Card
			if len(node.OnList) > 0 {
				card *= 0.1
			}
			node.Cost = &plan.Cost{
				Card: card,
			}

		case plan.Node_LEFT:
			card := leftCost.Card * rightCost.Card
			if len(node.OnList) > 0 {
				card *= 0.1
				card += leftCost.Card
			}
			node.Cost = &plan.Cost{
				Card: card,
			}

		case plan.Node_RIGHT:
			card := leftCost.Card * rightCost.Card
			if len(node.OnList) > 0 {
				card *= 0.1
				card += rightCost.Card
			}
			node.Cost = &plan.Cost{
				Card: card,
			}

		case plan.Node_OUTER:
			card := leftCost.Card * rightCost.Card
			if len(node.OnList) > 0 {
				card *= 0.1
				card += leftCost.Card + rightCost.Card
			}
			node.Cost = &plan.Cost{
				Card: card,
			}

		case plan.Node_SEMI, plan.Node_ANTI:
			node.Cost = &plan.Cost{
				Card: leftCost.Card * .7,
			}

		case plan.Node_SINGLE, plan.Node_MARK:
			node.Cost = &plan.Cost{
				Card: leftCost.Card,
			}
		}

	case plan.Node_AGG:
		if len(node.GroupBy) > 0 {
			childCost := builder.qry.Nodes[node.Children[0]].Cost
			node.Cost = &plan.Cost{
				Card: childCost.Card * 0.1,
			}
		} else {
			node.Cost = &plan.Cost{
				Card: 1,
			}
		}

	default:
		if len(node.Children) > 0 {
			childCost := builder.qry.Nodes[node.Children[0]].Cost
			node.Cost = &plan.Cost{
				Card: childCost.Card,
			}
		} else if node.Cost == nil {
			node.Cost = &plan.Cost{
				Card: 1,
			}
		}
	}

	return nodeID
}

func (builder *QueryBuilder) rewriteRightJoinToLeftJoin(nodeID int32) {
	node := builder.qry.Nodes[nodeID]
	if node.NodeType == plan.Node_JOIN {
		builder.rewriteRightJoinToLeftJoin(node.Children[0])
		builder.rewriteRightJoinToLeftJoin(node.Children[1])

		if node.JoinType == plan.Node_RIGHT {
			node.JoinType = plan.Node_LEFT
			node.Children = []int32{node.Children[1], node.Children[0]}
		}
	} else if len(node.Children) > 0 {
		builder.rewriteRightJoinToLeftJoin(node.Children[0])
	}
}

func (builder *QueryBuilder) buildFrom(stmt tree.TableExprs, ctx *BindContext) (int32, error) {
	if len(stmt) == 1 {
		return builder.buildTable(stmt[0], ctx)
	}

	leftCtx := NewBindContext(builder, ctx)
	leftChildID, err := builder.buildTable(stmt[0], leftCtx)
	if err != nil {
		return 0, err
	}

	for i := 1; i < len(stmt); i++ {
		rightCtx := NewBindContext(builder, ctx)
		rightChildID, err := builder.buildTable(stmt[i], rightCtx)
		if err != nil {
			return 0, err
		}

		leftChildID = builder.appendNode(&plan.Node{
			NodeType: plan.Node_JOIN,
			Children: []int32{leftChildID, rightChildID},
			JoinType: plan.Node_INNER,
		}, nil)

		if i == len(stmt)-1 {
			builder.ctxByNode[leftChildID] = ctx
			err = ctx.mergeContexts(leftCtx, rightCtx)
			if err != nil {
				return 0, err
			}
		} else {
			newCtx := NewBindContext(builder, ctx)
			builder.ctxByNode[leftChildID] = newCtx
			err = newCtx.mergeContexts(leftCtx, rightCtx)
			if err != nil {
				return 0, err
			}
			leftCtx = newCtx
		}
	}

	return leftChildID, err
}

func (builder *QueryBuilder) buildTable(stmt tree.TableExpr, ctx *BindContext) (nodeID int32, err error) {
	switch tbl := stmt.(type) {
	case *tree.Select:
		subCtx := NewBindContext(builder, ctx)
		nodeID, err = builder.buildSelect(tbl, subCtx, false)
		if subCtx.isCorrelated {
			return 0, moerr.NewNYI("correlated subquery in FROM clause")
		}

		if subCtx.hasSingleRow {
			ctx.hasSingleRow = true
		}

	case *tree.TableName:
		schema := string(tbl.SchemaName)
		table := string(tbl.ObjectName)
		if len(table) == 0 || table == "dual" { //special table name
			nodeID = builder.appendNode(&plan.Node{
				NodeType: plan.Node_VALUE_SCAN,
			}, ctx)

			ctx.hasSingleRow = true

			break
		}

		if len(schema) == 0 {
			cteRef := ctx.findCTE(table)
			if cteRef != nil {
				subCtx := NewBindContext(builder, ctx)
				subCtx.maskedCTEs = cteRef.maskedCTEs
				subCtx.cteName = table
				//reset defaultDatabase
				if len(cteRef.defaultDatabase) > 0 {
					subCtx.defaultDatabase = cteRef.defaultDatabase
				}

				switch stmt := cteRef.ast.Stmt.(type) {
				case *tree.Select:
					nodeID, err = builder.buildSelect(stmt, subCtx, false)

				case *tree.ParenSelect:
					nodeID, err = builder.buildSelect(stmt.Select, subCtx, false)

				default:
					err = moerr.NewParseError("unexpected statement: '%v'", tree.String(stmt, dialect.MYSQL))
				}

				if err != nil {
					return
				}

				if subCtx.isCorrelated {
					return 0, moerr.NewNYI("correlated column in CTE")
				}

				if subCtx.hasSingleRow {
					ctx.hasSingleRow = true
				}

				cols := cteRef.ast.Name.Cols

				if len(cols) > len(subCtx.headings) {
					return 0, moerr.NewSyntaxError("table %q has %d columns available but %d columns specified", table, len(subCtx.headings), len(cols))
				}

				for i, col := range cols {
					subCtx.headings[i] = string(col)
				}

				break
			}
			schema = ctx.defaultDatabase
		}

		obj, tableDef := builder.compCtx.Resolve(schema, table)
		if tableDef == nil {
			return 0, moerr.NewParseError("table %q does not exist", table)
		}

		tableDef.Name2ColIndex = map[string]int32{}
		for i := 0; i < len(tableDef.Cols); i++ {
			tableDef.Name2ColIndex[tableDef.Cols[i].Name] = int32(i)
		}
		nodeType := plan.Node_TABLE_SCAN
		if tableDef.TableType == catalog.SystemExternalRel {
			nodeType = plan.Node_EXTERNAL_SCAN
		} else if tableDef.TableType == catalog.SystemViewRel {

			// set view statment to CTE
			viewDefString := ""
			for _, def := range tableDef.Defs {
				if viewDef, ok := def.Def.(*plan.TableDef_DefType_View); ok {
					viewDefString = viewDef.View.View
					break
				}
			}
			if viewDefString != "" {
				if ctx.cteByName == nil {
					ctx.cteByName = make(map[string]*CTERef)
				}

				viewData := ViewData{}
				err := json.Unmarshal([]byte(viewDefString), &viewData)
				if err != nil {
					return 0, err
				}

				originStmts, err := mysql.Parse(viewData.Stmt)
				if err != nil {
					return 0, err
				}
				viewStmt, ok := originStmts[0].(*tree.CreateView)
				if !ok {
					return 0, moerr.NewParseError("can not get view statement")
				}

				viewName := viewStmt.Name.ObjectName
				var maskedCTEs map[string]any
				if len(ctx.cteByName) > 0 {
					maskedCTEs := make(map[string]any)
					for name := range ctx.cteByName {
						maskedCTEs[name] = nil
					}
				}

				ctx.cteByName[string(viewName)] = &CTERef{
					ast: &tree.CTE{
						Name: &tree.AliasClause{
							Alias: tree.Identifier(viewName),
							Cols:  viewStmt.ColNames,
						},
						Stmt: viewStmt.AsSource,
					},
					defaultDatabase: viewData.DefaultDatabase,
					maskedCTEs:      maskedCTEs,
				}

				newTableName := tree.NewTableName(tree.Identifier(viewName), tree.ObjectNamePrefix{
					CatalogName:     tbl.CatalogName, // TODO unused now, if used in some code, that will be save in view
					SchemaName:      tree.Identifier(""),
					ExplicitCatalog: false,
					ExplicitSchema:  false,
				})
				return builder.buildTable(newTableName, ctx)
			}
		}

		nodeID = builder.appendNode(&plan.Node{
			NodeType:    nodeType,
			Cost:        builder.compCtx.Cost(obj, nil),
			ObjRef:      obj,
			TableDef:    tableDef,
			BindingTags: []int32{builder.genNewTag()},
		}, ctx)

	case *tree.JoinTableExpr:
		return builder.buildJoinTable(tbl, ctx)

	case *tree.ParenTableExpr:
		return builder.buildTable(tbl.Expr, ctx)

	case *tree.AliasedTableExpr: //allways AliasedTableExpr first
		if _, ok := tbl.Expr.(*tree.Select); ok {
			if tbl.As.Alias == "" {
				return 0, moerr.NewSyntaxError("subquery in FROM must have an alias: %T", stmt)
			}
		}

		nodeID, err = builder.buildTable(tbl.Expr, ctx)
		if err != nil {
			return
		}

		err = builder.addBinding(nodeID, tbl.As, ctx)

		return

	case *tree.StatementSource:
		return 0, moerr.NewParseError("unsupport table expr: %T", stmt)

	default:
		// Values table not support
		return 0, moerr.NewParseError("unsupport table expr: %T", stmt)
	}

	return
}

func (builder *QueryBuilder) genNewTag() int32 {
	builder.nextTag++
	return builder.nextTag
}

func (builder *QueryBuilder) addBinding(nodeID int32, alias tree.AliasClause, ctx *BindContext) error {
	node := builder.qry.Nodes[nodeID]

	if node.NodeType == plan.Node_VALUE_SCAN {
		return nil
	}

	var cols []string
	var types []*plan.Type
	var binding *Binding

	if node.NodeType == plan.Node_TABLE_SCAN || node.NodeType == plan.Node_MATERIAL_SCAN || node.NodeType == plan.Node_EXTERNAL_SCAN {
		if len(alias.Cols) > len(node.TableDef.Cols) {
			return moerr.NewSyntaxError("table %q has %d columns available but %d columns specified", alias.Alias, len(node.TableDef.Cols), len(alias.Cols))
		}

		var table string
		if alias.Alias != "" {
			table = string(alias.Alias)
		} else {
			table = node.TableDef.Name
		}

		if _, ok := ctx.bindingByTable[table]; ok {
			return moerr.NewSyntaxError("table name %q specified more than once", table)
		}

		cols = make([]string, len(node.TableDef.Cols))
		types = make([]*plan.Type, len(node.TableDef.Cols))

		tag := node.BindingTags[0]

		for i, col := range node.TableDef.Cols {
			if i < len(alias.Cols) {
				cols[i] = string(alias.Cols[i])
			} else {
				cols[i] = col.Name
			}
			types[i] = col.Typ

			name := table + "." + cols[i]
			builder.nameByColRef[[2]int32{tag, int32(i)}] = name
		}

		binding = NewBinding(tag, nodeID, table, cols, types)
	} else {
		// Subquery
		subCtx := builder.ctxByNode[nodeID]
		headings := subCtx.headings
		projects := subCtx.projects

		if len(alias.Cols) > len(headings) {
			return moerr.NewSyntaxError("table %q has %d columns available but %d columns specified", alias.Alias, len(headings), len(alias.Cols))
		}

		table := subCtx.cteName
		if len(alias.Alias) > 0 {
			table = string(alias.Alias)
		}
		if _, ok := ctx.bindingByTable[table]; ok {
			return moerr.NewSyntaxError("table name %q specified more than once", table)
		}

		cols = make([]string, len(headings))
		types = make([]*plan.Type, len(headings))

		tag := builder.ctxByNode[nodeID].rootTag()

		for i, col := range headings {
			if i < len(alias.Cols) {
				cols[i] = string(alias.Cols[i])
			} else {
				cols[i] = col
			}
			types[i] = projects[i].Typ

			name := table + "." + cols[i]
			builder.nameByColRef[[2]int32{tag, int32(i)}] = name
		}

		binding = NewBinding(tag, nodeID, table, cols, types)
	}

	ctx.bindings = append(ctx.bindings, binding)
	ctx.bindingByTag[binding.tag] = binding
	ctx.bindingByTable[binding.table] = binding

	for _, col := range cols {
		if _, ok := ctx.bindingByCol[col]; ok {
			ctx.bindingByCol[col] = nil
		} else {
			ctx.bindingByCol[col] = binding
		}
	}

	ctx.bindingTree = &BindingTreeNode{
		binding: binding,
	}

	return nil
}

func (builder *QueryBuilder) buildJoinTable(tbl *tree.JoinTableExpr, ctx *BindContext) (int32, error) {
	var joinType plan.Node_JoinFlag

	switch tbl.JoinType {
	case tree.JOIN_TYPE_CROSS, tree.JOIN_TYPE_INNER, tree.JOIN_TYPE_NATURAL:
		joinType = plan.Node_INNER
	case tree.JOIN_TYPE_LEFT, tree.JOIN_TYPE_NATURAL_LEFT:
		joinType = plan.Node_LEFT
	case tree.JOIN_TYPE_RIGHT, tree.JOIN_TYPE_NATURAL_RIGHT:
		joinType = plan.Node_RIGHT
	case tree.JOIN_TYPE_FULL:
		joinType = plan.Node_OUTER
	}

	leftCtx := NewBindContext(builder, ctx)
	rightCtx := NewBindContext(builder, ctx)

	leftChildID, err := builder.buildTable(tbl.Left, leftCtx)
	if err != nil {
		return 0, err
	}

	rightChildID, err := builder.buildTable(tbl.Right, rightCtx)
	if err != nil {
		return 0, err
	}

	err = ctx.mergeContexts(leftCtx, rightCtx)
	if err != nil {
		return 0, err
	}

	nodeID := builder.appendNode(&plan.Node{
		NodeType: plan.Node_JOIN,
		Children: []int32{leftChildID, rightChildID},
		JoinType: joinType,
	}, ctx)
	node := builder.qry.Nodes[nodeID]

	ctx.binder = NewTableBinder(builder, ctx)

	switch cond := tbl.Cond.(type) {
	case *tree.OnJoinCond:
		joinConds, err := splitAndBindCondition(cond.Expr, ctx)
		if err != nil {
			return 0, err
		}

		node.OnList = joinConds

	case *tree.UsingJoinCond:
		for _, col := range cond.Cols {
			expr, err := ctx.addUsingCol(string(col), joinType, leftCtx, rightCtx)
			if err != nil {
				return 0, err
			}

			node.OnList = append(node.OnList, expr)
		}

	default:
		if tbl.JoinType == tree.JOIN_TYPE_NATURAL || tbl.JoinType == tree.JOIN_TYPE_NATURAL_LEFT || tbl.JoinType == tree.JOIN_TYPE_NATURAL_RIGHT {
			leftCols := make(map[string]any)
			for _, binding := range leftCtx.bindings {
				for _, col := range binding.cols {
					leftCols[col] = nil
				}
			}

			var usingCols []string
			for _, binding := range rightCtx.bindings {
				for _, col := range binding.cols {
					if _, ok := leftCols[col]; ok {
						usingCols = append(usingCols, col)
					}
				}
			}

			for _, col := range usingCols {
				expr, err := ctx.addUsingCol(col, joinType, leftCtx, rightCtx)
				if err != nil {
					return 0, err
				}

				node.OnList = append(node.OnList, expr)
			}
		}
	}

	return nodeID, nil
}

func (builder *QueryBuilder) pushdownFilters(nodeID int32, filters []*plan.Expr) (int32, []*plan.Expr) {
	node := builder.qry.Nodes[nodeID]

	var canPushdown, cantPushdown []*plan.Expr

	switch node.NodeType {
	case plan.Node_AGG:
		groupTag := node.BindingTags[0]
		aggregateTag := node.BindingTags[1]

		for _, filter := range filters {
			if !containsTag(filter, aggregateTag) {
				canPushdown = append(canPushdown, replaceColRefs(filter, groupTag, node.GroupBy))
			} else {
				cantPushdown = append(cantPushdown, filter)
			}
		}

		childID, cantPushdownChild := builder.pushdownFilters(node.Children[0], canPushdown)

		if len(cantPushdownChild) > 0 {
			childID = builder.appendNode(&plan.Node{
				NodeType:   plan.Node_FILTER,
				Children:   []int32{node.Children[0]},
				FilterList: cantPushdownChild,
			}, nil)
		}

		node.Children[0] = childID

	case plan.Node_FILTER:
		canPushdown = filters
		for _, filter := range node.FilterList {
			canPushdown = append(canPushdown, splitPlanConjunction(applyDistributivity(filter))...)
		}

		childID, cantPushdownChild := builder.pushdownFilters(node.Children[0], canPushdown)

		if len(cantPushdownChild) > 0 {
			node.Children[0] = childID
			node.FilterList = cantPushdownChild
		} else {
			nodeID = childID
		}

	case plan.Node_JOIN:
		leftTags := make(map[int32]*Binding)
		for _, tag := range builder.enumerateTags(node.Children[0]) {
			leftTags[tag] = nil
		}

		rightTags := make(map[int32]*Binding)
		for _, tag := range builder.enumerateTags(node.Children[1]) {
			rightTags[tag] = nil
		}

		if node.JoinType == plan.Node_INNER {
			for _, cond := range node.OnList {
				filters = append(filters, splitPlanConjunction(applyDistributivity(cond))...)
			}

			node.OnList = nil
		}

		var leftPushdown, rightPushdown []*plan.Expr
		var turnInner bool

		joinSides := make([]int8, len(filters))

		for i, filter := range filters {
			canTurnInner := true

			joinSides[i] = getJoinSide(filter, leftTags, rightTags)
			if f, ok := filter.Expr.(*plan.Expr_F); ok {
				for _, arg := range f.F.Args {
					if getJoinSide(arg, leftTags, rightTags) == JoinSideBoth {
						canTurnInner = false
						break
					}
				}
			}

			if joinSides[i]&JoinSideRight != 0 && canTurnInner && node.JoinType == plan.Node_LEFT && rejectsNull(filter) {
				for _, cond := range node.OnList {
					filters = append(filters, splitPlanConjunction(applyDistributivity(cond))...)
				}

				node.JoinType = plan.Node_INNER
				node.OnList = nil
				turnInner = true

				break
			}

			// TODO: FULL OUTER join should be handled here. However we don't have FULL OUTER join now.
		}

		if turnInner {
			joinSides = make([]int8, len(filters))

			for i, filter := range filters {
				joinSides[i] = getJoinSide(filter, leftTags, rightTags)
			}
		} else if node.JoinType == plan.Node_LEFT {
			var newOnList []*plan.Expr
			for _, cond := range node.OnList {
				conj := splitPlanConjunction(applyDistributivity(cond))
				for _, conjElem := range conj {
					side := getJoinSide(conjElem, leftTags, rightTags)
					if side&JoinSideLeft == 0 {
						rightPushdown = append(rightPushdown, conjElem)
					} else {
						newOnList = append(newOnList, conjElem)
					}
				}
			}

			node.OnList = newOnList
		}

		for i, filter := range filters {
			switch joinSides[i] {
			case JoinSideNone:
				if c, ok := filter.Expr.(*plan.Expr_C); ok {
					if c, ok := c.C.Value.(*plan.Const_Bval); ok {
						if c.Bval {
							break
						}
					}
				}

				switch node.JoinType {
				case plan.Node_INNER:
					leftPushdown = append(leftPushdown, DeepCopyExpr(filter))
					rightPushdown = append(rightPushdown, filter)

				case plan.Node_LEFT, plan.Node_SEMI, plan.Node_ANTI, plan.Node_SINGLE:
					leftPushdown = append(leftPushdown, filter)

				default:
					cantPushdown = append(cantPushdown, filter)
				}

			case JoinSideLeft:
				if node.JoinType != plan.Node_OUTER {
					leftPushdown = append(leftPushdown, filter)
				} else {
					cantPushdown = append(cantPushdown, filter)
				}

			case JoinSideRight:
				if node.JoinType == plan.Node_INNER {
					rightPushdown = append(rightPushdown, filter)
				} else {
					cantPushdown = append(cantPushdown, filter)
				}

			case JoinSideBoth:
				if node.JoinType == plan.Node_INNER {
					if f, ok := filter.Expr.(*plan.Expr_F); ok {
						if f.F.Func.ObjName == "=" {
							if getJoinSide(f.F.Args[0], leftTags, rightTags) != JoinSideBoth {
								if getJoinSide(f.F.Args[1], leftTags, rightTags) != JoinSideBoth {
									node.OnList = append(node.OnList, filter)
									break
								}
							}
						}
					}
				}

				cantPushdown = append(cantPushdown, filter)
			}
		}

		childID, cantPushdownChild := builder.pushdownFilters(node.Children[0], leftPushdown)

		if len(cantPushdownChild) > 0 {
			childID = builder.appendNode(&plan.Node{
				NodeType:   plan.Node_FILTER,
				Children:   []int32{node.Children[0]},
				FilterList: cantPushdownChild,
			}, nil)
		}

		node.Children[0] = childID

		childID, cantPushdownChild = builder.pushdownFilters(node.Children[1], rightPushdown)

		if len(cantPushdownChild) > 0 {
			childID = builder.appendNode(&plan.Node{
				NodeType:   plan.Node_FILTER,
				Children:   []int32{node.Children[1]},
				FilterList: cantPushdownChild,
			}, nil)
		}

		node.Children[1] = childID

	case plan.Node_PROJECT:
		child := builder.qry.Nodes[node.Children[0]]
		if (child.NodeType == plan.Node_VALUE_SCAN || child.NodeType == plan.Node_EXTERNAL_SCAN) && child.RowsetData == nil {
			cantPushdown = filters
			break
		}

		projectTag := node.BindingTags[0]

		for _, filter := range filters {
			canPushdown = append(canPushdown, replaceColRefs(filter, projectTag, node.ProjectList))
		}

		childID, cantPushdownChild := builder.pushdownFilters(node.Children[0], canPushdown)

		if len(cantPushdownChild) > 0 {
			childID = builder.appendNode(&plan.Node{
				NodeType:   plan.Node_FILTER,
				Children:   []int32{node.Children[0]},
				FilterList: cantPushdownChild,
			}, nil)
		}

		node.Children[0] = childID

	case plan.Node_TABLE_SCAN, plan.Node_EXTERNAL_SCAN:
		node.FilterList = append(node.FilterList, filters...)

	default:
		if len(node.Children) > 0 {
			childID, cantPushdownChild := builder.pushdownFilters(node.Children[0], filters)

			if len(cantPushdownChild) > 0 {
				childID = builder.appendNode(&plan.Node{
					NodeType:   plan.Node_FILTER,
					Children:   []int32{node.Children[0]},
					FilterList: cantPushdownChild,
				}, nil)
			}

			node.Children[0] = childID
		} else {
			cantPushdown = filters
		}
	}

	return nodeID, cantPushdown
}
