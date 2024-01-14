package taskrunner

import (
	"context"
	"testing"

	"github.com/ChenJesse/taskrunner/shell"
	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	mockRunWithFlags := func(ctx context.Context, shellRun shell.ShellRun, flags map[string]FlagArg) error {
		return nil
	}

	testCases := []struct {
		description string
		shouldPanic bool
		mockTask    *Task
		panicVal    string
	}{
		{
			description: "Should panic if both Run and RunWithFlags is defined",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Run:          func(ctx context.Context, shellRun shell.ShellRun) error { return nil },
			},
			panicVal: "Both `Run` and `RunWithFlags` are defined on the `mock/task/1` task. `Run` is deprecated, please prefer using `RunWithFlags`.",
		},
		{
			description: "Should panic if task Name matches flag LongName",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						LongName:  "mock/task/1",
						ValueType: BoolTypeFlag,
					},
				},
			},
			panicVal: "Task name `mock/task/1` and flag LongName `mock/task/1` are currently duplicates. Please de-depulicate their names.",
		},
		{
			description: "Should panic if task Name matches flag ShortName",
			mockTask: &Task{
				Name:         "a",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						ShortName: []rune("a")[0],
						ValueType: BoolTypeFlag,
					},
				},
			},
			panicVal: "Task name `a` and flag ShortName `a` are currently duplicates. Please de-depulicate their names.",
		},
		{
			description: "Should panic if a flag is missing a description",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						LongName:  "flagA",
						ValueType: BoolTypeFlag,
					},
				},
			},
			panicVal: "Please provide a description for the `flagA` flag.",
		},
		{
			description: "Should panic if a flag is missing a ValueType",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						LongName:    "flagA",
					},
				},
			},
			panicVal: "Please set the flag ValueType with `taskrunner.StringTypeFlag`, `taskrunner.BoolTypeFlag`, `taskrunner.DurationTypeFlag`, `taskrunner.Float64TypeFlag` or `taskrunner.IntTypeFlag`.",
		},
		{
			description: "Should panic if a flag has a ValueType that does not match XTypeFlag",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						LongName:    "flagA",
						ValueType:   "thisIsInvalid",
					},
				},
			},
			panicVal: "Please set the flag ValueType with `taskrunner.StringTypeFlag`, `taskrunner.BoolTypeFlag`, `taskrunner.DurationTypeFlag`, `taskrunner.Float64TypeFlag` or `taskrunner.IntTypeFlag`.",
		},
		{
			description: "Should panic if a flag lacks both a LongName and ShortName",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						ValueType:   BoolTypeFlag,
					},
				},
			},
			panicVal: "Neither LongName nor ShortName are defined for a flag on `mock/task/1`. At least one name must be defined.",
		},
		{
			description: "Should panic if there are duplicate flag LongNames",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						LongName:    "flagA",
						ValueType:   BoolTypeFlag,
					},
					{
						Description: "This is a description of flagA",
						LongName:    "flagA",
						ValueType:   BoolTypeFlag,
					},
				},
			},
			panicVal: "Duplicate flag LongName registered: `flagA`.",
		},
		{
			description: "Should panic if a there are duplicate flag ShortNames",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						ShortName:   []rune("a")[0],
						ValueType:   BoolTypeFlag,
					},
					{
						Description: "This is a description of flagA",
						ShortName:   []rune("a")[0],
						ValueType:   BoolTypeFlag,
					},
				},
			},
			panicVal: "Duplicate flag ShortName registered: `a`.",
		},
		{
			description: "Should panic if reserved `help` long name is used",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						LongName:    "help",
						ValueType:   BoolTypeFlag,
					},
				},
			},
			panicVal: "The LongFlag name `help` is reserved.",
		},
		{
			description: "Should panic if reserved `h` short name is used",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						ShortName:   []rune("h")[0],
						ValueType:   BoolTypeFlag,
					},
				},
			},
			panicVal: "The ShortFlag name `h` is reserved.",
		},
		{
			description: "Should succeed if Description, valid LongName and ValueType exist",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						LongName:    "flagA",
						ValueType:   BoolTypeFlag,
					},
				},
			},
		},
		{
			description: "Should succeed if Description, valid ShortName and ValueType exist",
			mockTask: &Task{
				Name:         "mock/task/1",
				RunWithFlags: mockRunWithFlags,
				Flags: []TaskFlag{
					{
						Description: "This is a description of flagA",
						ShortName:   []rune("a")[0],
						ValueType:   BoolTypeFlag,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		registry := NewRegistry()
		t.Run(tc.description, func(t *testing.T) {
			if tc.panicVal != "" {
				assert.PanicsWithValue(t, tc.panicVal, func() { registry.Add(tc.mockTask) })
			} else {
				assert.NotPanics(t, func() { registry.Add(tc.mockTask) })
			}
		})
	}
}
