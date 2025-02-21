/*
Copyright 2022 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"knative.dev/pkg/apis"
	logtesting "knative.dev/pkg/logging/testing"
)

func TestPipeline_Validate_Success(t *testing.T) {
	tests := []struct {
		name string
		p    *Pipeline
		wc   func(context.Context) context.Context
	}{{
		name: "valid metadata",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}}},
			},
		},
	}, {
		name: "pipelinetask custom task references",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{APIVersion: "example.dev/v0", Kind: "Example", Name: ""}}},
			},
		},
		wc: enableFeatures(t, []string{"enable-custom-tasks"}),
	}, {
		name: "do not validate spec on delete",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
		},
		wc: apis.WithinDelete,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.wc != nil {
				ctx = tt.wc(ctx)
			}
			err := tt.p.Validate(ctx)
			if err != nil {
				t.Errorf("Pipeline.Validate() returned error for valid Pipeline: %v", err)
			}
		})
	}
}

func TestPipeline_Validate_Failure(t *testing.T) {
	tests := []struct {
		name          string
		p             *Pipeline
		expectedError apis.FieldError
		wc            func(context.Context) context.Context
	}{{
		name: "comma in name",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipe,line"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid resource name "pipe,line": must be a valid DNS label`,
			Paths:   []string{"metadata.name"},
		},
	}, {
		name: "pipeline name too long",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "asdf123456789012345678901234567890123456789012345678901234567890"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}}},
			},
		},
		expectedError: apis.FieldError{
			Message: "Invalid resource name: length must be no more than 63 characters",
			Paths:   []string{"metadata.name"},
		},
	}, {
		name: "pipeline spec missing",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
		},
		expectedError: apis.FieldError{
			Message: `expected at least one, got none`,
			Paths:   []string{"spec.description", "spec.params", "spec.resources", "spec.tasks", "spec.workspaces"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.wc != nil {
				ctx = tt.wc(ctx)
			}
			err := tt.p.Validate(ctx)
			if err == nil {
				t.Error("Pipeline.Validate() did not return error for invalid pipeline")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("Pipeline.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineSpec_Validate_Failure(t *testing.T) {
	tests := []struct {
		name          string
		ps            *PipelineSpec
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline with one pipeline task having taskRef and taskSpec both",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:     "invalid-pipeline-task",
				TaskRef:  &TaskRef{Name: "foo-task"},
				TaskSpec: &EmbeddedTask{TaskSpec: getTaskSpec()},
			}},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"tasks[1].taskRef", "tasks[1].taskSpec"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid operator (not In/NotIn)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "bar-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "exists" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid operator (not In/NotIn)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "bar-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "bar-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "exists" is not recognized. valid operators: in,notin`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid operator (not In/NotIn)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "bar-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "bar-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.Exists,
					Values:   []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "exists" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid values (empty)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid values (empty)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid values (empty)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
					Values:   []string{},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid operator (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid operator (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "" is not recognized. valid operators: in,notin`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid operator (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:  "foo",
					Values: []string{"foo"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: operator "" is not recognized. valid operators: in,notin`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with invalid values (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with invalid values (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with invalid values (missing)",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "foo",
					Operator: selection.In,
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expecting non-empty values field`,
			Paths:   []string{"tasks[0].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having when expression with misconfigured result reference",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
			Paths:   []string{"tasks[1].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having when expression with misconfigured result reference",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having when expression with misconfigured result reference",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When: []WhenExpression{{
					Input:    "$(tasks.a-task.resultTypo.bResult)",
					Operator: selection.In,
					Values:   []string{"bar"},
				}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
			Paths:   []string{"tasks[1].when[0]", "finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with one pipeline task having blank when expression",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When:    []WhenExpression{{}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `missing field(s)`,
			Paths:   []string{"tasks[1].when[0]"},
		},
	}, {
		name: "invalid pipeline with final task having blank when expression",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When:    []WhenExpression{{}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `missing field(s)`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid pipeline with dag task and final task having blank when expression",
		ps: &PipelineSpec{
			Description: "this is an invalid pipeline with invalid pipeline task",
			Tasks: []PipelineTask{{
				Name:    "valid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
			}, {
				Name:    "invalid-pipeline-task",
				TaskRef: &TaskRef{Name: "foo-task"},
				When:    []WhenExpression{{}},
			}},
			Finally: []PipelineTask{{
				Name:    "invalid-pipeline-task-finally",
				TaskRef: &TaskRef{Name: "foo-task"},
				When:    []WhenExpression{{}},
			}},
		},
		expectedError: apis.FieldError{
			Message: `missing field(s)`,
			Paths:   []string{"tasks[1].when[0]", "finally[0].when[0]"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := config.SkipValidationDueToPropagatedParametersAndWorkspaces(context.Background(), false)
			err := tt.ps.Validate(ctx)
			if err == nil {
				t.Errorf("PipelineSpec.Validate() did not return error for invalid pipelineSpec")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineSpec_Validate_Failure_CycleDAG(t *testing.T) {
	name := "invalid pipeline spec with DAG having cyclic dependency"
	ps := &PipelineSpec{
		Tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}, RunAfter: []string{"baz"},
		}, {
			Name: "bar", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"foo"},
		}, {
			Name: "baz", TaskRef: &TaskRef{Name: "baz-task"}, RunAfter: []string{"bar"},
		}},
	}
	ctx := config.SkipValidationDueToPropagatedParametersAndWorkspaces(context.Background(), false)
	err := ps.Validate(ctx)
	if err == nil {
		t.Errorf("PipelineSpec.Validate() did not return error for invalid pipelineSpec: %s", name)
	}
}

func TestValidatePipelineTasks_Failure(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		finalTasks    []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "pipeline tasks invalid (duplicate tasks)",
		tasks: []PipelineTask{
			{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}},
			{Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"tasks[1].name"},
		},
	}, {
		name: "apiVersion with steps",
		tasks: []PipelineTask{{
			Name: "foo",
			TaskSpec: &EmbeddedTask{
				TypeMeta: runtime.TypeMeta{
					APIVersion: "tekton.dev/v1",
				},
				TaskSpec: TaskSpec{
					Steps: []Step{{
						Name:  "some-step",
						Image: "some-image",
					}},
				},
			},
		}},
		finalTasks: nil,
		expectedError: apis.FieldError{
			Message: "taskSpec.apiVersion cannot be specified when using taskSpec.steps",
			Paths:   []string{"tasks[0].taskSpec.apiVersion"},
		},
	}, {
		name: "kind with steps",
		tasks: []PipelineTask{{
			Name: "foo",
			TaskSpec: &EmbeddedTask{
				TypeMeta: runtime.TypeMeta{
					Kind: "Task",
				},
				TaskSpec: TaskSpec{
					Steps: []Step{{
						Name:  "some-step",
						Image: "some-image",
					}},
				},
			},
		}},
		finalTasks: nil,
		expectedError: apis.FieldError{
			Message: "taskSpec.kind cannot be specified when using taskSpec.steps",
			Paths:   []string{"tasks[0].taskSpec.kind"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := config.SkipValidationDueToPropagatedParametersAndWorkspaces(context.Background(), false)
			err := ValidatePipelineTasks(ctx, tt.tasks, tt.finalTasks)
			if err == nil {
				t.Error("ValidatePipelineTasks() did not return error for invalid pipeline tasks")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("ValidatePipelineTasks() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidateGraph_Success(t *testing.T) {
	desc := "valid dependency graph with multiple tasks"
	tasks := []PipelineTask{{
		Name: "foo", TaskRef: &TaskRef{Name: "foo-task"},
	}, {
		Name: "bar", TaskRef: &TaskRef{Name: "bar-task"},
	}, {
		Name: "foo1", TaskRef: &TaskRef{Name: "foo-task"}, RunAfter: []string{"foo"},
	}, {
		Name: "bar1", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"bar"},
	}, {
		Name: "foo-bar", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"foo1", "bar1"},
	}}
	if err := validateGraph(tasks); err != nil {
		t.Errorf("Pipeline.validateGraph() returned error for valid DAG of pipeline tasks: %s: %v", desc, err)
	}
}

func TestValidateGraph_Failure(t *testing.T) {
	desc := "invalid dependency graph between the tasks with cyclic dependency"
	tasks := []PipelineTask{{
		Name: "foo", TaskRef: &TaskRef{Name: "foo-task"}, RunAfter: []string{"bar"},
	}, {
		Name: "bar", TaskRef: &TaskRef{Name: "bar-task"}, RunAfter: []string{"foo"},
	}}
	expectedError := apis.FieldError{
		Message: `invalid value: cycle detected; task "bar" depends on "foo"`,
		Paths:   []string{"tasks"},
	}
	err := validateGraph(tasks)
	if err == nil {
		t.Error("Pipeline.validateGraph() did not return error for invalid DAG of pipeline tasks:", desc)
	} else if d := cmp.Diff(expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
		t.Errorf("Pipeline.validateGraph() errors diff %s", diff.PrintWantGot(d))
	}
}

func TestValidateParamResults_Success(t *testing.T) {
	desc := "valid pipeline task referencing task result along with parameter variable"
	tasks := []PipelineTask{{
		TaskSpec: &EmbeddedTask{TaskSpec: TaskSpec{
			Results: []TaskResult{{
				Name: "output",
			}},
			Steps: []Step{{
				Name: "foo", Image: "bar",
			}},
		}},
		Name: "a-task",
	}, {
		Name:    "foo",
		TaskRef: &TaskRef{Name: "foo-task"},
		Params: []Param{{
			Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.foo) and $(tasks.a-task.results.output)"},
		}},
	}}
	if err := validateParamResults(tasks); err != nil {
		t.Errorf("Pipeline.validateParamResults() returned error for valid pipeline: %s: %v", desc, err)
	}
}

func TestValidateParamResults_Failure(t *testing.T) {
	desc := "invalid pipeline task referencing task results with malformed variable substitution expression"
	tasks := []PipelineTask{{
		Name: "a-task", TaskRef: &TaskRef{Name: "a-task"},
	}, {
		Name: "b-task", TaskRef: &TaskRef{Name: "b-task"},
		Params: []Param{{
			Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.a-task.resultTypo.bResult)"}}},
	}}
	expectedError := apis.FieldError{
		Message: `invalid value: expected all of the expressions [tasks.a-task.resultTypo.bResult] to be result expressions but only [] were`,
		Paths:   []string{"tasks[1].params[a-param].value"},
	}
	err := validateParamResults(tasks)
	if err == nil {
		t.Errorf("Pipeline.validateParamResults() did not return error for invalid pipeline: %s", desc)
	}
	if d := cmp.Diff(expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
		t.Errorf("Pipeline.validateParamResults() errors diff %s", diff.PrintWantGot(d))
	}
}

func TestValidatePipelineResults_Success(t *testing.T) {
	desc := "valid pipeline with valid pipeline results syntax"
	results := []PipelineResult{{
		Name:        "my-pipeline-result",
		Description: "this is my pipeline result",
		Value:       *NewStructuredValues("$(tasks.a-task.results.output)"),
	}, {
		Name:        "my-pipeline-object-result",
		Description: "this is my pipeline result",
		Value:       *NewStructuredValues("$(tasks.a-task.results.gitrepo.commit)"),
	}}
	if err := validatePipelineResults(results, []PipelineTask{{Name: "a-task"}}); err != nil {
		t.Errorf("Pipeline.validatePipelineResults() returned error for valid pipeline: %s: %v", desc, err)
	}
}

func TestValidatePipelineResults_Failure(t *testing.T) {
	tests := []struct {
		desc          string
		results       []PipelineResult
		expectedError apis.FieldError
	}{{
		desc: "invalid pipeline result reference",
		results: []PipelineResult{{
			Name:        "my-pipeline-result",
			Description: "this is my pipeline result",
			Value:       *NewStructuredValues("$(tasks.a-task.results.output.key1.extra)"),
		}},
		expectedError: *apis.ErrInvalidValue(`expected all of the expressions [tasks.a-task.results.output.key1.extra] to be result expressions but only [] were`, "results[0].value").Also(
			apis.ErrInvalidValue("referencing a nonexistent task", "results[0].value")),
	}, {
		desc: "invalid pipeline result value with static string",
		results: []PipelineResult{{
			Name:        "my-pipeline-result",
			Description: "this is my pipeline result",
			Value:       *NewStructuredValues("foo.bar"),
		}},
		expectedError: *apis.ErrInvalidValue(`expected pipeline results to be task result expressions but an invalid expressions was found`, "results[0].value").Also(
			apis.ErrInvalidValue(`expected pipeline results to be task result expressions but no expressions were found`, "results[0].value")).Also(
			apis.ErrInvalidValue(`referencing a nonexistent task`, "results[0].value")),
	}, {
		desc: "invalid pipeline result value with invalid expression",
		results: []PipelineResult{{
			Name:        "my-pipeline-result",
			Description: "this is my pipeline result",
			Value:       *NewStructuredValues("$(foo.bar)"),
		}},
		expectedError: *apis.ErrInvalidValue(`expected pipeline results to be task result expressions but an invalid expressions was found`, "results[0].value").Also(
			apis.ErrInvalidValue("referencing a nonexistent task", "results[0].value")),
	}}
	for _, tt := range tests {
		err := validatePipelineResults(tt.results, []PipelineTask{{Name: "a-task"}})
		if err == nil {
			t.Errorf("Pipeline.validatePipelineResults() did not return for invalid pipeline: %s", tt.desc)
		}
		if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
			t.Errorf("Pipeline.validatePipelineResults() errors diff %s", diff.PrintWantGot(d))
		}
	}
}

func TestFinallyTaskResultsToPipelineResults_Success(t *testing.T) {
	tests := []struct {
		name string
		p    *Pipeline
		wc   func(context.Context) context.Context
	}{{
		name: "valid pipeline with pipeline results",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Results: []PipelineResult{{
					Name:  "initialized",
					Value: *NewStructuredValues("$(tasks.clone-app-repo.results.initialized)"),
				}},
				Tasks: []PipelineTask{{
					Name: "clone-app-repo",
					TaskSpec: &EmbeddedTask{TaskSpec: TaskSpec{
						Results: []TaskResult{{
							Name: "initialized",
							Type: "string",
						}},
						Steps: []Step{{
							Name: "foo", Image: "bar",
						}},
					}},
				}},
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.wc != nil {
				ctx = tt.wc(ctx)
			}
			err := tt.p.Validate(ctx)
			if err != nil {
				t.Errorf("Pipeline.finallyTaskResultsToPipelineResults() returned error for valid Pipeline: %v", err)
			}
		})
	}
}

func TestFinallyTaskResultsToPipelineResults_Failure(t *testing.T) {
	tests := []struct {
		desc          string
		p             *Pipeline
		expectedError apis.FieldError
		wc            func(context.Context) context.Context
	}{{
		desc: "invalid propagation of finally task results from pipeline results",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Results: []PipelineResult{{
					Name:  "initialized",
					Value: *NewStructuredValues("$(tasks.check-git-commit.results.init)"),
				}},
				Tasks: []PipelineTask{{
					Name: "clone-app-repo",
					TaskSpec: &EmbeddedTask{TaskSpec: TaskSpec{
						Results: []TaskResult{{
							Name: "current-date-unix-timestamp",
							Type: "string",
						}},
						Steps: []Step{{
							Name: "foo", Image: "bar",
						}},
					}},
				}},
				Finally: []PipelineTask{{
					Name: "check-git-commit",
					TaskSpec: &EmbeddedTask{TaskSpec: TaskSpec{
						Results: []TaskResult{{
							Name: "init",
							Type: "string",
						}},
						Steps: []Step{{
							Name: "foo2", Image: "bar",
						}},
					}},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: referencing a nonexistent task`,
			Paths:   []string{"spec.results[0].value"},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.Background()
			if tt.wc != nil {
				ctx = tt.wc(ctx)
			}
			err := tt.p.Validate(ctx)
			if err == nil {
				t.Errorf("Pipeline.finallyTaskResultsToPipelineResults() did not return for invalid pipeline: %s", tt.desc)
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("Pipeline.finallyTaskResultsToPipelineResults() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidatePipelineParameterVariables_Success(t *testing.T) {
	tests := []struct {
		name   string
		params []ParamSpec
		tasks  []PipelineTask
	}{{
		name: "valid string parameter variables",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "foo-is-baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.baz) and $(params.foo-is-baz)"},
			}},
		}},
	}, {
		name: "valid string parameter variables in when expression",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "foo-is-baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.baz)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}, {
				Input:    "baz",
				Operator: selection.In,
				Values:   []string{"$(params.foo-is-baz)"},
			}},
		}},
	}, {
		name: "valid string parameter variables in input, array reference in values in when expression",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "foo", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.baz)",
				Operator: selection.In,
				Values:   []string{"$(params.foo[*])"},
			}},
		}},
	}, {
		name: "valid array parameter variables",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"some", "default"}},
		}, {
			Name: "foo-is-baz", Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"$(params.baz)", "and", "$(params.foo-is-baz)"}},
			}},
		}},
	}, {
		name: "valid star array parameter variables",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"some", "default"}},
		}, {
			Name: "foo-is-baz", Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"$(params.baz[*])", "and", "$(params.foo-is-baz[*])"}},
			}},
		}},
	}, {
		name: "pipeline parameter nested in task parameter",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(input.workspace.$(params.baz))"},
			}},
		}},
	}, {
		name: "array param - using the whole variable as a param's value that is intended to be array type",
		params: []ParamSpec{{
			Name: "myArray",
			Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param-intended-to-be-array", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.myArray[*])"},
			}},
		}},
	}, {
		name: "object param - using single individual variable in string param",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-string-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.myObject.key1)"},
			}},
		}},
	}, {
		name: "object param - using multiple individual variables in string param",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-string-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.myObject.key1) and $(params.myObject.key2)"},
			}},
		}},
	}, {
		name: "object param - using individual variables in array param",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "an-array-param", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"$(params.myObject.key1)", "another one $(params.myObject.key2)"}},
			}},
		}},
	}, {
		name: "object param - using individual variables and string param as the value of other object individual keys",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}, {
			Name: "myString",
			Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "an-object-param", Value: ParamValue{Type: ParamTypeObject, ObjectVal: map[string]string{
					"url":    "$(params.myObject.key1)",
					"commit": "$(params.myString)",
				}},
			}},
		}},
	}, {
		name: "object param - using the whole variable as a param's value that is intended to be object type",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param-intended-to-be-object", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.myObject[*])"},
			}},
		}},
	}, {
		name: "object param - using individual variable in input of when expression, and using both object individual variable and array reference in values of when expression",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}, {
			Name: "foo", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.myObject.key1)",
				Operator: selection.In,
				Values:   []string{"$(params.foo[*])", "$(params.myObject.key2)"},
			}},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := config.EnableAlphaAPIFields(context.Background())
			err := validatePipelineParameterVariables(ctx, tt.tasks, tt.params)
			if err != nil {
				t.Errorf("Pipeline.validatePipelineParameterVariables() returned error for valid pipeline parameters: %v", err)
			}
		})
	}
}

func TestValidatePipelineParameterVariables_Failure(t *testing.T) {
	tests := []struct {
		name          string
		params        []ParamSpec
		tasks         []PipelineTask
		expectedError apis.FieldError
		api           string
	}{{
		name: "invalid pipeline task with a parameter which is missing from the param declarations",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.does-not-exist)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.does-not-exist)"`,
			Paths:   []string{"[0].params[a-param]"},
		},
	}, {
		name: "invalid string parameter variables in when expression, missing input param from the param declarations",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.baz)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.baz)"`,
			Paths:   []string{"[0].when[0].input"},
		},
	}, {
		name: "invalid string parameter variables in when expression, missing values param from the param declarations",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "bax",
				Operator: selection.In,
				Values:   []string{"$(params.foo-is-baz)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.foo-is-baz)"`,
			Paths:   []string{"[0].when[0].values"},
		},
	}, {
		name: "invalid string parameter variables in when expression, array reference in input",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.foo)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `variable type invalid in "$(params.foo)"`,
			Paths:   []string{"[0].when[0].input"},
		},
	}, {
		name: "Invalid array parameter variable in when expression, array reference in input with array notation [*]",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.foo)[*]",
				Operator: selection.In,
				Values:   []string{"$(params.foo[*])"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `variable type invalid in "$(params.foo)[*]"`,
			Paths:   []string{"[0].when[0].input"},
		},
	}, {
		name: "invalid pipeline task with a parameter combined with missing param from the param declarations",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo-task",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.foo) and $(params.does-not-exist)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.foo) and $(params.does-not-exist)"`,
			Paths:   []string{"[0].params[a-param]"},
		},
	}, {
		name: "invalid pipeline task with two parameters and one of them missing from the param declarations",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo-task",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.foo)"},
			}, {
				Name: "b-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.does-not-exist)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.does-not-exist)"`,
			Paths:   []string{"[0].params[b-param]"},
		},
	}, {
		name: "invalid parameter type",
		params: []ParamSpec{{
			Name: "foo", Type: "invalidtype",
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalidtype`,
			Paths:   []string{"params.foo.type"},
		},
	}, {
		name: "array parameter mismatching default type",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeArray, Default: &ParamValue{Type: ParamTypeString, StringVal: "astring"},
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `"array" type does not match default value's type: "string"`,
			Paths:   []string{"params.foo.default.type", "params.foo.type"},
		},
	}, {
		name: "string parameter mismatching default type",
		params: []ParamSpec{{
			Name: "foo", Type: ParamTypeString, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params.foo.default.type", "params.foo.type"},
		},
	}, {
		name: "array parameter used as string",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.baz)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params.baz.default.type", "params.baz.type"},
		},
	}, {
		name: "star array parameter used as string",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.baz[*])"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params.baz.default.type", "params.baz.type"},
		},
	}, {
		name: "array parameter string template not isolated",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"value: $(params.baz)", "last"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params.baz.default.type", "params.baz.type"},
		},
	}, {
		name: "star array parameter string template not isolated",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString, Default: &ParamValue{Type: ParamTypeArray, ArrayVal: []string{"anarray", "elements"}},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"value: $(params.baz[*])", "last"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `"string" type does not match default value's type: "array"`,
			Paths:   []string{"params.baz.default.type", "params.baz.type"},
		},
	}, {
		name: "multiple string parameters with the same name",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeString,
		}, {
			Name: "baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `parameter appears more than once`,
			Paths:   []string{"params[baz]"},
		},
	}, {
		name: "multiple array parameters with the same name",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray,
		}, {
			Name: "baz", Type: ParamTypeArray,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `parameter appears more than once`,
			Paths:   []string{"params[baz]"},
		},
	}, {
		name: "multiple different type parameters with the same name",
		params: []ParamSpec{{
			Name: "baz", Type: ParamTypeArray,
		}, {
			Name: "baz", Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
		}},
		expectedError: apis.FieldError{
			Message: `parameter appears more than once`,
			Paths:   []string{"params[baz]"},
		},
	}, {
		name: "invalid object key in the input of the when expression",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "$(params.myObject.non-exist-key)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.myObject.non-exist-key)"`,
			Paths:   []string{"[0].when[0].input"},
		},
		api: "alpha",
	}, {
		name: "invalid object key in the Values of the when expression",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			When: []WhenExpression{{
				Input:    "bax",
				Operator: selection.In,
				Values:   []string{"$(params.myObject.non-exist-key)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.myObject.non-exist-key)"`,
			Paths:   []string{"[0].when[0].values"},
		},
		api: "alpha",
	}, {
		name: "invalid object key is used to provide values for array params",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"$(params.myObject.non-exist-key)", "last"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.myObject.non-exist-key)"`,
			Paths:   []string{"[0].params[a-param].value[0]"},
		},
		api: "alpha",
	}, {
		name: "invalid object key is used to provide values for string params",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "a-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.myObject.non-exist-key)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.myObject.non-exist-key)"`,
			Paths:   []string{"[0].params[a-param]"},
		},
		api: "alpha",
	}, {
		name: "invalid object key is used to provide values for object params",
		params: []ParamSpec{{
			Name: "myObject",
			Type: ParamTypeObject,
			Properties: map[string]PropertySpec{
				"key1": {Type: "string"},
				"key2": {Type: "string"},
			},
		}, {
			Name: "myString",
			Type: ParamTypeString,
		}},
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "an-object-param", Value: ParamValue{Type: ParamTypeObject, ObjectVal: map[string]string{
					"url":    "$(params.myObject.non-exist-key)",
					"commit": "$(params.myString)",
				}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.myObject.non-exist-key)"`,
			Paths:   []string{"[0].params[an-object-param].properties[url]"},
		},
		api: "alpha",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.api == "alpha" {
				ctx = config.EnableAlphaAPIFields(context.Background())
			}
			err := validatePipelineParameterVariables(ctx, tt.tasks, tt.params)
			if err == nil {
				t.Errorf("Pipeline.validatePipelineParameterVariables() did not return error for invalid pipeline parameters")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidatePipelineWorkspacesDeclarations_Success(t *testing.T) {
	desc := "pipeline spec workspaces do not cause an error"
	workspaces := []PipelineWorkspaceDeclaration{{
		Name: "foo",
	}, {
		Name: "bar",
	}}
	t.Run(desc, func(t *testing.T) {
		err := validatePipelineWorkspacesDeclarations(workspaces)
		if err != nil {
			t.Errorf("Pipeline.validatePipelineWorkspacesDeclarations() returned error for valid pipeline workspaces: %v", err)
		}
	})
}

func TestValidatePipelineWorkspacesUsage_Success(t *testing.T) {
	tests := []struct {
		name       string
		workspaces []PipelineWorkspaceDeclaration
		tasks      []PipelineTask
	}{{
		name: "unused pipeline spec workspaces do not cause an error",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "foo",
		}, {
			Name: "bar",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
		}},
	}, {
		name: "valid mapping pipeline-task workspace name with pipeline workspace name",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "pipelineWorkspaceName",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
			Workspaces: []WorkspacePipelineTaskBinding{{
				Name:      "pipelineWorkspaceName",
				Workspace: "",
			}},
		}},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validatePipelineWorkspacesUsage(tt.workspaces, tt.tasks).ViaField("tasks")
			if errs != nil {
				t.Errorf("Pipeline.validatePipelineWorkspacesUsage() returned error for valid pipeline workspaces: %v", errs)
			}
		})
	}
}

func TestValidatePipelineWorkspacesDeclarations_Failure(t *testing.T) {
	tests := []struct {
		name          string
		workspaces    []PipelineWorkspaceDeclaration
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "multiple workspaces sharing the same name are not allowed",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "foo",
		}, {
			Name: "foo",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: workspace with name "foo" appears more than once`,
			Paths:   []string{"workspaces[1]"},
		},
	}, {
		name: "workspace name must not be empty",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: workspace 0 has empty name`,
			Paths:   []string{"workspaces[0]"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validatePipelineWorkspacesDeclarations(tt.workspaces)
			if errs == nil {
				t.Errorf("Pipeline.validatePipelineWorkspacesDeclarations() did not return error for invalid pipeline workspaces")
			}
			if d := cmp.Diff(tt.expectedError.Error(), errs.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.validatePipelineWorkspacesDeclarations() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidatePipelineWorkspacesUsage_Failure(t *testing.T) {
	tests := []struct {
		name          string
		workspaces    []PipelineWorkspaceDeclaration
		tasks         []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "workspace bindings relying on a non-existent pipeline workspace cause an error",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "foo",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
			Workspaces: []WorkspacePipelineTaskBinding{{
				Name:      "taskWorkspaceName",
				Workspace: "pipelineWorkspaceName",
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task "foo" expects workspace with name "pipelineWorkspaceName" but none exists in pipeline spec`,
			Paths:   []string{"tasks[0].workspaces[0]"},
		},
	}, {
		name: "invalid mapping workspace with different name",
		workspaces: []PipelineWorkspaceDeclaration{{
			Name: "pipelineWorkspaceName",
		}},
		tasks: []PipelineTask{{
			Name: "foo", TaskRef: &TaskRef{Name: "foo"},
			Workspaces: []WorkspacePipelineTaskBinding{{
				Name:      "taskWorkspaceName",
				Workspace: "",
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task "foo" expects workspace with name "taskWorkspaceName" but none exists in pipeline spec`,
			Paths:   []string{"tasks[0].workspaces[0]"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validatePipelineWorkspacesUsage(tt.workspaces, tt.tasks).ViaField("tasks")
			if errs == nil {
				t.Errorf("Pipeline.validatePipelineWorkspacesUsage() did not return error for invalid pipeline workspaces")
			}
			if d := cmp.Diff(tt.expectedError.Error(), errs.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.validatePipelineWorkspacesUsage() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidatePipelineWithFinalTasks_Success(t *testing.T) {
	tests := []struct {
		name string
		p    *Pipeline
	}{{
		name: "valid pipeline with final tasks",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
				}, {
					Name:     "final-task-2",
					TaskSpec: &EmbeddedTask{TaskSpec: getTaskSpec()},
				}},
			},
		},
	}, {
		name: "valid pipeline with final tasks referring to task results from a dag task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "param1", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.non-final-task.results.output)"},
					}},
				}},
			},
		},
	}, {
		name: "valid pipeline with final tasks referring to context variables",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "param1", Value: ParamValue{Type: ParamTypeString, StringVal: "$(context.pipelineRun.name)"},
					}},
				}},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(context.Background())
			if err != nil {
				t.Errorf("Pipeline.Validate() returned error for valid pipeline with finally: %v", err)
			}
		})
	}
}

func TestValidatePipelineWithFinalTasks_Failure(t *testing.T) {
	tests := []struct {
		name          string
		p             *Pipeline
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline without any non-final task (tasks set to nil) but at least one final task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: nil,
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: spec.tasks is empty but spec.finally has 1 tasks`,
			Paths:   []string{"spec.finally"},
		},
	}, {
		name: "invalid pipeline without any non-final task (tasks set to empty list of pipeline task) but at least one final task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{}},
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: *apis.ErrMissingOneOf("spec.tasks[0].taskRef", "spec.tasks[0].taskSpec").Also(
			&apis.FieldError{
				Message: `invalid value ""`,
				Paths:   []string{"spec.tasks[0].name"},
				Details: "Pipeline Task name must be a valid DNS Label." +
					"For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
			}),
	}, {
		name: "invalid pipeline with valid non-final tasks but empty finally section",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{}},
			},
		},
		expectedError: *apis.ErrMissingOneOf("spec.finally[0].taskRef", "spec.finally[0].taskSpec").Also(
			&apis.FieldError{
				Message: `invalid value ""`,
				Paths:   []string{"spec.finally[0].name"},
				Details: "Pipeline Task name must be a valid DNS Label." +
					"For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
			}),
	}, {
		name: "invalid pipeline with duplicate final tasks",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}, {
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"spec.finally[1].name"},
		},
	}, {
		name: "invalid pipeline with same task name for final and non final task",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "common-task-name",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "common-task-name",
					TaskRef: &TaskRef{Name: "final-task"},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"spec.finally[0].name"},
		},
	}, {
		name: "final task missing taskref and taskspec",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name: "final-task",
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got neither`,
			Paths:   []string{"spec.finally[0].taskRef", "spec.finally[0].taskSpec"},
		},
	}, {
		name: "final task with both tasfref and taskspec",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:     "final-task",
					TaskRef:  &TaskRef{Name: "non-final-task"},
					TaskSpec: &EmbeddedTask{TaskSpec: getTaskSpec()},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `expected exactly one, got both`,
			Paths:   []string{"spec.finally[0].taskRef", "spec.finally[0].taskSpec"},
		},
	}, {
		name: "extra parameter called final-param provided to final task which is not specified in the Pipeline",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Params: []ParamSpec{{
					Name: "foo", Type: ParamTypeString,
				}},
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "final-param", Value: ParamValue{Type: ParamTypeString, StringVal: "$(params.foo) and $(params.does-not-exist)"},
					}},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(params.foo) and $(params.does-not-exist)"`,
			Paths:   []string{"spec.finally[0].params[final-param]"},
		},
	}, {
		name: "invalid pipeline with invalid final tasks with runAfter",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:     "final-task-1",
					TaskRef:  &TaskRef{Name: "final-task"},
					RunAfter: []string{"non-final-task"},
				}},
			},
		},
		expectedError: *apis.ErrGeneric("").Also(&apis.FieldError{
			Message: `invalid value: no runAfter allowed under spec.finally, final task final-task-1 has runAfter specified`,
			Paths:   []string{"spec.finally[0]"},
		}),
	}, {
		name: "invalid pipeline - workspace bindings in final task relying on a non-existent pipeline workspace",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name: "non-final-task", TaskRef: &TaskRef{Name: "foo"},
				}},
				Finally: []PipelineTask{{
					Name: "final-task", TaskRef: &TaskRef{Name: "foo"},
					Workspaces: []WorkspacePipelineTaskBinding{{
						Name:      "shared-workspace",
						Workspace: "pipeline-shared-workspace",
					}},
				}},
				Workspaces: []WorkspacePipelineDeclaration{{
					Name: "foo",
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task "final-task" expects workspace with name "pipeline-shared-workspace" but none exists in pipeline spec`,
			Paths:   []string{"spec.finally[0].workspaces[0]"},
		},
	}, {
		name: "invalid pipeline with no tasks under tasks section and empty finally section",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Finally: []PipelineTask{},
			},
		},
		expectedError: *apis.ErrGeneric("expected at least one, got none", "spec.description", "spec.params", "spec.resources", "spec.tasks", "spec.workspaces"),
	}, {
		name: "invalid pipeline with final tasks referring to invalid context variables",
		p: &Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "pipeline"},
			Spec: PipelineSpec{
				Tasks: []PipelineTask{{
					Name:    "non-final-task",
					TaskRef: &TaskRef{Name: "non-final-task"},
				}},
				Finally: []PipelineTask{{
					Name:    "final-task-1",
					TaskRef: &TaskRef{Name: "final-task"},
					Params: []Param{{
						Name: "param1", Value: ParamValue{Type: ParamTypeString, StringVal: "$(context.pipelineRun.missing)"},
					}},
				}},
			},
		},
		expectedError: apis.FieldError{
			Message: `non-existent variable in "$(context.pipelineRun.missing)"`,
			Paths:   []string{"spec.finally.value"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate(context.Background())
			if err == nil {
				t.Errorf("Pipeline.Validate() did not return error for invalid pipeline with finally")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestValidateTasksAndFinallySection_Success(t *testing.T) {
	tests := []struct {
		name string
		ps   *PipelineSpec
	}{{
		name: "pipeline with tasks and final tasks",
		ps: &PipelineSpec{
			Tasks: []PipelineTask{{
				Name: "non-final-task", TaskRef: &TaskRef{Name: "foo"},
			}},
			Finally: []PipelineTask{{
				Name: "final-task", TaskRef: &TaskRef{Name: "foo"},
			}},
		},
	}, {
		name: "valid pipeline with tasks and finally section without any tasks",
		ps: &PipelineSpec{
			Tasks: []PipelineTask{{
				Name: "my-task", TaskRef: &TaskRef{Name: "foo"},
			}},
			Finally: nil,
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTasksAndFinallySection(tt.ps)
			if err != nil {
				t.Errorf("Pipeline.ValidateTasksAndFinallySection() returned error for valid pipeline with finally: %v", err)
			}
		})
	}
}

func TestValidateTasksAndFinallySection_Failure(t *testing.T) {
	desc := "invalid pipeline with empty tasks and a few final tasks"
	ps := &PipelineSpec{
		Tasks: nil,
		Finally: []PipelineTask{{
			Name: "final-task", TaskRef: &TaskRef{Name: "foo"},
		}},
	}
	expectedError := apis.FieldError{
		Message: `invalid value: spec.tasks is empty but spec.finally has 1 tasks`,
		Paths:   []string{"finally"},
	}
	err := validateTasksAndFinallySection(ps)
	if err == nil {
		t.Errorf("Pipeline.ValidateTasksAndFinallySection() did not return error for invalid pipeline with finally: %s", desc)
	}
	if d := cmp.Diff(expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
		t.Errorf("Pipeline.validateParamResults() errors diff %s", diff.PrintWantGot(d))
	}
}

func TestValidateFinalTasks_Failure(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		finalTasks    []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "invalid pipeline with final task specifying runAfter",
		finalTasks: []PipelineTask{{
			Name:     "final-task",
			TaskRef:  &TaskRef{Name: "final-task"},
			RunAfter: []string{"non-final-task"},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: no runAfter allowed under spec.finally, final task final-task has runAfter specified`,
			Paths:   []string{"finally[0]"},
		},
	}, {
		name: "invalid pipeline with final tasks having task results reference from a final task",
		finalTasks: []PipelineTask{{
			Name:    "final-task-1",
			TaskRef: &TaskRef{Name: "final-task"},
		}, {
			Name:    "final-task-2",
			TaskRef: &TaskRef{Name: "final-task"},
			Params: []Param{{
				Name: "param1", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.final-task-1.results.output)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalid task result reference, final task has task result reference from a final task final-task-1`,
			Paths:   []string{"finally[1].params[param1].value"},
		},
	}, {
		name: "invalid pipeline with final tasks having task results reference from a final task",
		finalTasks: []PipelineTask{{
			Name:    "final-task-1",
			TaskRef: &TaskRef{Name: "final-task"},
		}, {
			Name:    "final-task-2",
			TaskRef: &TaskRef{Name: "final-task"},
			When: WhenExpressions{{
				Input:    "$(tasks.final-task-1.results.output)",
				Operator: selection.In,
				Values:   []string{"result"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalid task result reference, final task has task result reference from a final task final-task-1`,
			Paths:   []string{"finally[1].when[0]"},
		},
	}, {
		name: "invalid pipeline with final tasks having task results reference from non existent dag task",
		finalTasks: []PipelineTask{{
			Name:    "final-task",
			TaskRef: &TaskRef{Name: "final-task"},
			Params: []Param{{
				Name: "param1", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.no-dag-task-1.results.output)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: invalid task result reference, final task has task result reference from a task no-dag-task-1 which is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[param1].value"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFinalTasks(tt.tasks, tt.finalTasks)
			if err == nil {
				t.Errorf("Pipeline.ValidateFinalTasks() did not return error for invalid pipeline")
			}
			if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
				t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineTasksExecutionStatus(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []PipelineTask
		finalTasks    []PipelineTask
		expectedError apis.FieldError
	}{{
		name: "valid string variable in finally accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name: "foo",
		}},
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.foo.status)"},
			}, {
				Name: "tasks-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.status)"},
			}},
			When: WhenExpressions{{
				Input:    "$(tasks.foo.status)",
				Operator: selection.In,
				Values:   []string{"Failure"},
			}, {
				Input:    "$(tasks.status)",
				Operator: selection.In,
				Values:   []string{"Success"},
			}},
		}},
	}, {
		name: "valid task result reference with status as a variable must not cause validation failure",
		tasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.foo.results.status)"},
			}},
			When: WhenExpressions{WhenExpression{
				Input:    "$(tasks.foo.results.status)",
				Operator: selection.In,
				Values:   []string{"Failure"},
			}},
		}},
	}, {
		name: "valid variable concatenated with extra string in finally accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name: "foo",
		}},
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ParamValue{Type: ParamTypeString, StringVal: "Execution status of foo is $(tasks.foo.status)."},
			}},
		}},
	}, {
		name: "valid variable concatenated with other param in finally accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name: "foo",
		}},
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "foo-status", Value: ParamValue{Type: ParamTypeString, StringVal: "Execution status of $(tasks.taskname) is $(tasks.foo.status)."},
			}},
		}},
	}, {
		name: "invalid string variable in dag task accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "bar-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.bar.status)"},
			}},
			When: WhenExpressions{WhenExpression{
				Input:    "$(tasks.bar.status)",
				Operator: selection.In,
				Values:   []string{"foo"},
			}},
		}},
		expectedError: *apis.ErrGeneric("").Also(&apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[bar-status].value", "tasks[0].when[0]"},
		}),
	}, {
		name: "invalid string variable in dag task accessing aggregate status of tasks",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "tasks-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.status)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[tasks-status].value"},
		},
	}, {
		name: "invalid variable concatenated with extra string in dag task accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "bar-status", Value: ParamValue{Type: ParamTypeString, StringVal: "Execution status of bar is $(tasks.bar.status)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[bar-status].value"},
		},
	}, {
		name: "invalid array variable in dag task accessing pipelineTask status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "bar-status", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"$(tasks.bar.status)"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[bar-status].value"},
		},
	}, {
		name: "invalid array variable in dag task accessing aggregate tasks status",
		tasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			Params: []Param{{
				Name: "tasks-status", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"$(tasks.status)"}},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline tasks can not refer to execution status of any other pipeline task or aggregate status of tasks`,
			Paths:   []string{"tasks[0].params[tasks-status].value"},
		},
	}, {
		name: "invalid string variable in finally accessing missing pipelineTask status",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.notask.status)"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value"},
		},
	}, {
		name: "invalid string variable in finally accessing missing pipelineTask status in when expression",
		finalTasks: []PipelineTask{{
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			When: WhenExpressions{{
				Input:    "$(tasks.notask.status)",
				Operator: selection.In,
				Values:   []string{"Success"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].when[0]"},
		},
	}, {
		name: "invalid string variable in finally accessing missing pipelineTask status in params and when expression",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ParamValue{Type: ParamTypeString, StringVal: "$(tasks.notask.status)"},
			}},
		}, {
			Name:    "foo",
			TaskRef: &TaskRef{Name: "foo-task"},
			When: WhenExpressions{{
				Input:    "$(tasks.notask.status)",
				Operator: selection.In,
				Values:   []string{"Success"},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value", "finally[1].when[0]"},
		},
	}, {
		name: "invalid variable concatenated with extra string in finally accessing missing pipelineTask status",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ParamValue{Type: ParamTypeString, StringVal: "Execution status of notask is $(tasks.notask.status)."},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value"},
		},
	}, {
		name: "invalid variable concatenated with other params in finally accessing missing pipelineTask status",
		finalTasks: []PipelineTask{{
			Name:    "bar",
			TaskRef: &TaskRef{Name: "bar-task"},
			Params: []Param{{
				Name: "notask-status", Value: ParamValue{Type: ParamTypeString, StringVal: "Execution status of $(tasks.taskname) is $(tasks.notask.status)."},
			}},
		}},
		expectedError: apis.FieldError{
			Message: `invalid value: pipeline task notask is not defined in the pipeline`,
			Paths:   []string{"finally[0].params[notask-status].value"},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecutionStatusVariables(tt.tasks, tt.finalTasks)
			if len(tt.expectedError.Error()) == 0 {
				if err != nil {
					t.Errorf("Pipeline.validateExecutionStatusVariables() returned error for valid pipeline variable accessing execution status: %s: %v", tt.name, err)
				}
			} else {
				if err == nil {
					t.Errorf("Pipeline.validateExecutionStatusVariables() did not return error for invalid pipeline parameters accessing execution status: %s, %s", tt.name, tt.tasks[0].Params)
				}
				if d := cmp.Diff(tt.expectedError.Error(), err.Error(), cmpopts.IgnoreUnexported(apis.FieldError{})); d != "" {
					t.Errorf("PipelineSpec.Validate() errors diff %s", diff.PrintWantGot(d))
				}
			}
		})
	}
}

func getTaskSpec() TaskSpec {
	return TaskSpec{
		Steps: []Step{{
			Name: "foo", Image: "bar",
		}},
	}
}

func enableFeatures(t *testing.T, features []string) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		s := config.NewStore(logtesting.TestLogger(t))
		data := make(map[string]string)
		for _, f := range features {
			data[f] = "true"
		}
		s.OnConfigChanged(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: config.GetFeatureFlagsConfigName()},
			Data:       data,
		})
		return s.ToContext(ctx)
	}
}
