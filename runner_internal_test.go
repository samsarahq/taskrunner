package taskrunner

import (
	"context"
	"testing"

	"github.com/samsarahq/taskrunner/shell"
	"github.com/stretchr/testify/assert"
)

func TestRunnerGroupTaskAndFlagArgs(t *testing.T) {
	mockRunWithFlags := func(ctx context.Context, shellRun shell.ShellRun, flags map[string]FlagArg) error {
		return nil
	}
	mockTask1 := &Task{
		Name:         "mock/task/1",
		RunWithFlags: mockRunWithFlags,
		Flags: []TaskFlag{
			{
				Description: "This is a description of what the bool flag A does.",
				LongName:    "longFlagA",
				ShortName:   []rune("a")[0],
				ValueType:   BoolTypeFlag,
			},
			{
				Description: "This is a description of what the bool flag B does.",
				LongName:    "longFlagB",
				ShortName:   []rune("b")[0],
				ValueType:   BoolTypeFlag,
			},
		},
	}
	mockTask2 := &Task{
		Name:         "mock/task/2",
		RunWithFlags: mockRunWithFlags,
		Flags: []TaskFlag{
			{
				Description: "This is a description of what string flag C does.",
				ShortName:   []rune("c")[0],
				ValueType:   StringTypeFlag,
			},
		},
	}
	mockTask3 := &Task{
		Name:         "mock/task/3",
		RunWithFlags: mockRunWithFlags,
		Flags: []TaskFlag{
			{
				Description: "This is a description of what int flag D does.",
				ShortName:   []rune("d")[0],
				ValueType:   IntTypeFlag,
			},
		},
	}

	testCases := []struct {
		description    string
		cliArgs        []string
		expectedGroups map[string][]string
	}{
		{
			description: "Should group single flag to single task",
			cliArgs:     []string{"mock/task/1", "--longFlagA"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longFlagA"},
			},
		},
		{
			description: "Should group multiple flags (long or short) to single task",
			cliArgs:     []string{"mock/task/1", "--longFlagA", "-b"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longFlagA", "-b"},
			},
		},
		{
			description: "Should group multiple flags to multiple tasks",
			cliArgs:     []string{"mock/task/1", "--longFlagA", "-b", "mock/task/2", "-c='test'", "mock/task/3", "-d=1"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longFlagA", "-b"},
				"mock/task/2": {"-c='test'"},
				"mock/task/3": {"-d=1"},
			},
		},
		{
			description: "Should exclude tasks passed directly to taskrunner",
			cliArgs:     []string{"--config", "mock/task/1", "--longFlagA"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longFlagA"},
			},
		},
		{
			// Validation for supported flags happens in the executor.
			description: "Should include unsupported flag args in group",
			cliArgs:     []string{"mock/task/1", "--longInvalidFlag"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longInvalidFlag"},
			},
		},
		{
			// Validation for supported tasks happens in the executor.
			description: "Should include unsupported task names in group",
			cliArgs:     []string{"thisisnotarealtask", "--longInvalidFlag"},
			expectedGroups: map[string][]string{
				"thisisnotarealtask": {"--longInvalidFlag"},
			},
		},
	}

	runner := newRuntime()
	runner.registry.Add(mockTask1)
	runner.registry.Add(mockTask2)
	runner.registry.Add(mockTask3)

	for _, tc := range testCases {
		taskFlagGroups := runner.groupTaskAndFlagArgs(tc.cliArgs)
		var expectedTaskGroups []string
		for key := range tc.expectedGroups {
			expectedTaskGroups = append(expectedTaskGroups, key)
		}

		var taskGroups []string
		for key, val := range taskFlagGroups {
			taskGroups = append(taskGroups, key)
			flags := tc.expectedGroups[key]
			assert.ElementsMatch(t, flags, val)
		}

		assert.ElementsMatch(t, expectedTaskGroups, taskGroups)
	}
}
