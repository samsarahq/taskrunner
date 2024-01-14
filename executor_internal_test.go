package taskrunner

import (
	"context"
	"testing"
	"time"

	"github.com/ChenJesse/taskrunner/config"
	"github.com/ChenJesse/taskrunner/shell"
	"github.com/stretchr/testify/assert"
)

func boolPtr(i bool) *bool {
	return &i
}

func intPtr(i int) *int {
	return &i
}

func float64Ptr(i float64) *float64 {
	return &i
}

func stringPtr(i string) *string {
	return &i
}

func durationPtr(i time.Duration) *time.Duration {
	return &i
}

func TestParseTaskOptionsListToMap(t *testing.T) {
	config := &config.Config{}
	mockRunWithFlags := func(ctx context.Context, shellRun shell.ShellRun, flags map[string]FlagArg) error {
		return nil
	}

	taskFlagA := TaskFlag{
		Description: "This is a description of what bool flag A does.",
		LongName:    "flagA",
		ShortName:   []rune("a")[0],
		ValueType:   BoolTypeFlag,
		Default:     "true",
	}
	taskFlagB := TaskFlag{
		Description: "This is a description of what string flag B does.",
		ShortName:   []rune("b")[0],
		ValueType:   StringTypeFlag,
		Default:     "presidential dolphin",
	}
	taskFlagC := TaskFlag{
		Description: "This is a description of what int flag C does.",
		ShortName:   []rune("c")[0],
		ValueType:   IntTypeFlag,
		Default:     "10",
	}
	taskFlagD := TaskFlag{
		Description: "This is a description of what float64 flag D does.",
		ShortName:   []rune("d")[0],
		ValueType:   Float64TypeFlag,
		Default:     "1.72",
	}
	taskFlagE := TaskFlag{
		Description: "This is a description of what duration flag E does.",
		ShortName:   []rune("e")[0],
		ValueType:   DurationTypeFlag,
		Default:     "11h",
	}
	taskFlagF := TaskFlag{
		Description: "This is a description of what duration flag F does.",
		ShortName:   []rune("f")[0],
		ValueType:   BoolTypeFlag,
	}
	mockTaskName := "mock/task/1"
	mockTask1 := &Task{
		Name:         mockTaskName,
		RunWithFlags: mockRunWithFlags,
		Flags:        []TaskFlag{taskFlagA, taskFlagB, taskFlagC, taskFlagD, taskFlagE},
	}
	mockTaskFlagRegistry := map[string]map[string]TaskFlag{
		mockTaskName: {
			"flagA": taskFlagA,
			"a":     taskFlagA,
			"b":     taskFlagB,
			"c":     taskFlagC,
			"d":     taskFlagD,
			"e":     taskFlagE,
			"f":     taskFlagF,
		},
	}

	tasks := []*Task{mockTask1}
	testCases := []struct {
		description          string
		flagArgs             []string
		expectedFlagAVal     *bool
		expectedLongFlagAVal *bool
		expectedFlagBVal     *string
		expectedFlagCVal     *int
		expectedFlagDVal     *float64
		expectedFlagEVal     *time.Duration
		expectFlagFValIsNil  bool
		invalidFlagPanicVal  string
	}{
		{
			description:          "Should parse BoolTypeFlags correctly and include value for both Short and Long Names",
			flagArgs:             []string{"-a"},
			expectedFlagAVal:     boolPtr(true),
			expectedLongFlagAVal: boolPtr(true),
		},
		{
			description:      "Should parse String/Duration/Int/Float64TypeFlags wrapped in \"\"s correctly",
			flagArgs:         []string{"-b=\"test\"", "-c=\"1\"", "-d=\"1.3\"", "-e=\"10h\""},
			expectedFlagBVal: stringPtr("test"),
			expectedFlagCVal: intPtr(1),
			expectedFlagDVal: float64Ptr(1.3),
			expectedFlagEVal: durationPtr(time.Duration(10 * time.Hour)),
		},
		{
			description:      "Should parse String/Duration/Int/Float64TypeFlags not wrapped in \"\"s correctly",
			flagArgs:         []string{"-b=test", "-c=1", "-d=1.3", "-e=10h"},
			expectedFlagBVal: stringPtr("test"),
			expectedFlagCVal: intPtr(1),
			expectedFlagDVal: float64Ptr(1.3),
			expectedFlagEVal: durationPtr(time.Duration(10 * time.Hour)),
		},
		{
			description:      "Should respect Default str value if no arg passed to variable flag",
			flagArgs:         []string{"-b"},
			expectedFlagBVal: stringPtr("presidential dolphin"),
		},
		{
			description:      "Should respect Default int value if no arg passed to variable flag",
			flagArgs:         []string{"-c"},
			expectedFlagCVal: intPtr(10),
		},
		{
			description:      "Should respect Default float64 value if no arg passed to variable flag",
			flagArgs:         []string{"-d"},
			expectedFlagDVal: float64Ptr(1.72),
		},
		{
			description:      "Should respect Default duration value if no arg passed to variable flag",
			flagArgs:         []string{"-e"},
			expectedFlagEVal: durationPtr(time.Duration(11 * time.Hour)),
		},
		{
			description:         "Should return nil if no arg passed to variable flag and no Default defined",
			flagArgs:            []string{"-f"},
			expectFlagFValIsNil: true,
		},
		{
			description:          "Should store Default values for all flags and Value nil when no args are passed",
			flagArgs:             []string{},
			expectedFlagAVal:     boolPtr(true),
			expectedLongFlagAVal: boolPtr(true),
			expectedFlagBVal:     stringPtr("presidential dolphin"),
			expectedFlagCVal:     intPtr(10),
			expectedFlagDVal:     float64Ptr(1.72),
			expectFlagFValIsNil:  true,
		},
		{
			description:         "Should panic if there are multiple `=`s in the flag",
			flagArgs:            []string{"-b=\"testing\"=\"testing123\""},
			invalidFlagPanicVal: "Invalid flag syntax for mock/task/1: `-b=\"testing\"=\"testing123\"`",
		},
		{
			description:         "Should panic if an unsupported flag is passed",
			flagArgs:            []string{"--invalidFlag"},
			invalidFlagPanicVal: "Unsupported flag passed to mock/task/1: `--invalidFlag`. See error: Unsupported flag: --invalidFlag",
		},
		{
			description: "Should panic if not possible to cast string to int for IntTypeFlag args",
			flagArgs:    []string{"-c=\"\"00\"10\"b\"\""},
		},
		{
			description: "Should panic if not possible to cast string to float64 for Float64TypeFlag args",
			flagArgs:    []string{"-d=\"\"00\"10.3\"b\"\""},
		},
		{
			description: "Should panic if not possible to cast string to duration for DurationTypeFlag args",
			flagArgs:    []string{"-e=\"\"00\"10.3\"b\"\"09m"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			executorOptions := []ExecutorOption{
				func(e *Executor) {
					e.taskFlagArgs = map[string][]string{
						mockTaskName: tc.flagArgs,
					}
				},
				func(e *Executor) {
					e.taskFlagsRegistry = mockTaskFlagRegistry
				},
			}

			executor := NewExecutor(config, tasks, executorOptions...)
			if tc.invalidFlagPanicVal != "" {
				assert.PanicsWithValue(t, tc.invalidFlagPanicVal, func() { executor.parseTaskFlagsIntoMap(mockTaskName, tc.flagArgs) })
			} else {
				flagsMap := executor.parseTaskFlagsIntoMap(mockTaskName, tc.flagArgs)
				if tc.expectedFlagAVal != nil {
					assert.Equal(t, *tc.expectedFlagAVal, *flagsMap["a"].BoolVal())
					assert.Equal(t, *tc.expectedLongFlagAVal, *flagsMap["flagA"].BoolVal())

					flagArgShort := flagsMap["a"]
					assert.Panics(t, func() { flagArgShort.IntVal() })
					assert.Panics(t, func() { flagArgShort.StringVal() })
					assert.Panics(t, func() { flagArgShort.Float64Val() })
					assert.Panics(t, func() { flagArgShort.DurationVal() })

					flagArgLong := flagsMap["flagA"]
					assert.Panics(t, func() { flagArgLong.IntVal() })
					assert.Panics(t, func() { flagArgLong.StringVal() })
					assert.Panics(t, func() { flagArgLong.Float64Val() })
					assert.Panics(t, func() { flagArgLong.DurationVal() })

					if len(tc.flagArgs) == 0 {
						assert.Nil(t, flagArgShort.Value)
						assert.Nil(t, flagArgLong.Value)
					}
				}

				if tc.expectedFlagBVal != nil {
					assert.Equal(t, *tc.expectedFlagBVal, *flagsMap["b"].StringVal())

					flagArg := flagsMap["b"]
					assert.Panics(t, func() { flagArg.IntVal() })
					assert.Panics(t, func() { flagArg.BoolVal() })
					assert.Panics(t, func() { flagArg.Float64Val() })
					assert.Panics(t, func() { flagArg.DurationVal() })

					if len(tc.flagArgs) == 0 {
						assert.Nil(t, flagArg.Value)
					}
				}

				if tc.expectedFlagCVal != nil {
					assert.Equal(t, *tc.expectedFlagCVal, *flagsMap["c"].IntVal())

					flagArg := flagsMap["c"]
					assert.Panics(t, func() { flagArg.StringVal() })
					assert.Panics(t, func() { flagArg.BoolVal() })
					assert.Panics(t, func() { flagArg.Float64Val() })
					assert.Panics(t, func() { flagArg.DurationVal() })

					if len(tc.flagArgs) == 0 {
						assert.Nil(t, flagArg.Value)
					}
				}

				if tc.expectedFlagDVal != nil {
					assert.Equal(t, *tc.expectedFlagDVal, *flagsMap["d"].Float64Val())

					flagArg := flagsMap["d"]
					assert.Panics(t, func() { flagArg.StringVal() })
					assert.Panics(t, func() { flagArg.BoolVal() })
					assert.Panics(t, func() { flagArg.IntVal() })
					assert.Panics(t, func() { flagArg.DurationVal() })

					if len(tc.flagArgs) == 0 {
						assert.Nil(t, flagArg.Value)
					}
				}

				if tc.expectedFlagEVal != nil {
					assert.Equal(t, *tc.expectedFlagEVal, *flagsMap["e"].DurationVal())

					flagArg := flagsMap["e"]
					assert.Panics(t, func() { flagArg.StringVal() })
					assert.Panics(t, func() { flagArg.BoolVal() })
					assert.Panics(t, func() { flagArg.IntVal() })
					assert.Panics(t, func() { flagArg.Float64Val() })

					if len(tc.flagArgs) == 0 {
						assert.Nil(t, flagArg.Value)
					}
				}

				if tc.expectFlagFValIsNil {
					flagArg := flagsMap["f"]
					assert.Nil(t, flagArg.BoolVal())
				}
			}
		})
	}
}
