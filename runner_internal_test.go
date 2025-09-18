package taskrunner

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/samsarahq/taskrunner/shell"
)

// MockRunnerLogger is a mock RunnerLogger implementation that
// tracks whether a fatal method was called. Not thread-safe, and
// a separate instance must be used for each test.
type MockRunnerLogger struct {
	fatal bool
}

func (l *MockRunnerLogger) Printf(_ string, _ ...interface{}) {}
func (l *MockRunnerLogger) Println(_ ...interface{})          {}
func (l *MockRunnerLogger) Fatalf(_ string, _ ...interface{}) {
	l.fatal = true
}
func (l *MockRunnerLogger) Fatalln(_ ...interface{}) {
	l.fatal = true
}
func (l *MockRunnerLogger) Writer() io.Writer {
	return io.Discard
}

func TestRunnerRun_FatalScenarios(t *testing.T) {
	// setup mock config and task
	testConfigFile := "config/testdata/base.taskrunner.json"
	mockRunWithFlags := func(_ context.Context, _ shell.ShellRun, _ map[string]FlagArg) error {
		return nil
	}
	mockTask := &Task{
		Name:         "basic", // must match a task defined in "desiredTasks" of the config file.
		RunWithFlags: mockRunWithFlags,
	}

	testCases := []struct {
		description   string
		tasks         []*Task
		cliArgs       []string // the first arg should be the command, which is irrelevant for testing.
		expectedFatal bool
	}{
		{
			description:   "Fatals on invalid config path",
			cliArgs:       []string{"", "--config", "unknownPath"},
			expectedFatal: true,
		},
		{
			description:   "Fatals on no registered tasks",
			cliArgs:       []string{"", "--config", testConfigFile},
			expectedFatal: true,
		},
		{
			description:   "Fatals when -list and -listAll are both specified",
			tasks:         []*Task{mockTask},
			cliArgs:       []string{"", "--config", testConfigFile, "--list", "--listAll"},
			expectedFatal: true,
		},
		{
			description:   "Fatals when unrecognized flag is specified",
			tasks:         []*Task{mockTask},
			cliArgs:       []string{"", "--unrecognizedFlag"},
			expectedFatal: true,
		},
		{
			description:   "Does not fatal when no tasks are specified",
			tasks:         []*Task{mockTask},
			cliArgs:       []string{"", "--config", testConfigFile, "--non-interactive"}, // non-interactive mode to ensure test exits when task completes
			expectedFatal: false,
		},
		{
			description:   "Fatals when task is specified but unknown",
			tasks:         []*Task{mockTask},
			cliArgs:       []string{"", "--config", testConfigFile, "unknown/task", "--someFlag"},
			expectedFatal: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// override logger
			mockLogger := &MockRunnerLogger{}
			logger = mockLogger

			// reset registry for each test
			DefaultRegistry = NewRegistry()
			for _, task := range tc.tasks {
				DefaultRegistry.Add(task)
			}

			// run command and assert fatal status
			os.Args = tc.cliArgs
			Run()
			assert.Equal(t, tc.expectedFatal, mockLogger.fatal, "fatal called")
		})
	}
}

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
		description               string
		cliArgs                   []string
		expectedGroups            map[string][]string
		expectedFoundUnknownTasks bool
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
			description: "Should exclude unsupported tasks and any flags passed to it",
			cliArgs:     []string{"mock/task/1", "--longFlagA", "thisisnotarealtask", "--longInvalidFlag", "mock/task/2", "-c='test'"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longFlagA"},
				"mock/task/2": {"-c='test'"},
			},
			expectedFoundUnknownTasks: true,
		},
		{
			// Validation for supported flags happens in the executor.
			description: "Should include unsupported flag args in group",
			cliArgs:     []string{"mock/task/1", "--longInvalidFlag"},
			expectedGroups: map[string][]string{
				"mock/task/1": {"--longInvalidFlag"},
			},
		},
	}

	runner := newRuntime()
	runner.registry.Add(mockTask1)
	runner.registry.Add(mockTask2)
	runner.registry.Add(mockTask3)

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			taskFlagGroups, foundUnknownTasks := runner.groupTaskAndFlagArgs(tc.cliArgs)
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
			assert.Equal(t, tc.expectedFoundUnknownTasks, foundUnknownTasks)
		})
	}
}
