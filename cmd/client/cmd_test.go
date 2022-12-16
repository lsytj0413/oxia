package client

import (
	"bytes"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"oxia/cmd/client/delete"
	"oxia/cmd/client/get"
	"oxia/cmd/client/list"
	"oxia/cmd/client/put"
	"oxia/server/kv"
	"oxia/server/wal"
	"oxia/standalone"
	"testing"
)

func TestClientCmd(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	kvOptions := kv.KVFactoryOptions{InMemory: true}
	kvFactory, err := kv.NewPebbleKVFactory(&kvOptions)
	assert.NoError(t, err)
	defer kvFactory.Close()
	walFactory := wal.NewInMemoryWalFactory()
	defer walFactory.Close()
	server, err := standalone.NewStandaloneRpcServer(0, "localhost", 1, walFactory, kvFactory)
	assert.NoError(t, err)

	serviceAddress := fmt.Sprintf("localhost:%d", server.Container.Port())

	stdin := bytes.NewBufferString("")
	stdout := bytes.NewBufferString("")
	stderr := bytes.NewBufferString("")
	Cmd.SetIn(stdin)
	Cmd.SetOut(stdout)
	Cmd.SetErr(stderr)

	for _, test := range []struct {
		name             string
		args             []string
		stdin            string
		expectedErr      error
		expectedStdOutRe string
		expectedStdErrRe string
	}{
		{"put", []string{"put", "-k", "k-put", "-p", "a"}, "", nil,
			"\\{\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"put-expected", []string{"put", "-k", "k-put", "-p", "c", "-e", "0"}, "", nil,
			"\\{\"stat\":\\{\"version\":1,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"put-unexpected-present", []string{"put", "-k", "k-put", "-p", "c", "-e", "0"}, "", nil,
			"\\{\"error\":\"unexpected version\"\\}",
			"^$",
		},
		{"put-unexpected-not-present", []string{"put", "-k", "k-put-unp", "-p", "c", "-e", "9999"}, "", nil,
			"\\{\"error\":\"unexpected version\"\\}",
			"^$",
		},
		{"put-multi", []string{"put", "-k", "2a", "-p", "c", "-k", "2x", "-p", "y"}, "", nil,
			"\\{\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}\\n{\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"put-binary", []string{"put", "-k", "k-put-binary-ok", "-p", "aGVsbG8y", "-b"}, "", nil,
			"\\{\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"put-binary-fail", []string{"put", "-k", "k-put-binary-fail", "-p", "not-binary", "-b"}, "", nil,
			"\\{\"error\":\"binary flag was set but payload is not valid base64\"\\}",
			"^$",
		},
		{"put-no-payload", []string{"put", "-k", "k-put-np"}, "", put.ErrorExpectedKeyPayloadInconsistent,
			".*",
			"Error: inconsistent flags; key and payload flags must be in pairs",
		},
		{"put-no-key", []string{"put", "-p", "k-put-np"}, "", put.ErrorExpectedKeyPayloadInconsistent,
			".*",
			"Error: inconsistent flags; key and payload flags must be in pairs",
		},
		{"put-stdin", []string{"put"}, "{\"key\":\"3a\",\"payload\":\"aGVsbG8y\",\"binary\":true}\n{\"key\":\"3x\",\"payload\":\"aGVsbG8y\"}", nil,
			"\\{\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}\\n{\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"put-bad-binary-use", []string{"put", "-b"}, "", put.ErrorIncorrectBinaryFlagUse,
			".*",
			"Error: binary flag was set when config is being sourced from stdin",
		},
		{"get", []string{"get", "-k", "k-put-binary-ok"}, "", nil,
			"\\{\"binary\":false,\"payload\":\"hello2\",\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"get-binary", []string{"get", "-k", "k-put-binary-ok", "-b"}, "", nil,
			"\\{\"binary\":true,\"payload\":\"aGVsbG8y\",\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"get-not-exist", []string{"get", "-k", "does-not-exist"}, "", nil,
			"\\{\"error\":\"key not found\"\\}",
			"^$",
		},
		{"get-multi", []string{"get", "-k", "2a", "-k", "2x"}, "", nil,
			"\\{\"binary\":false,\"payload\":\"c\",\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}\\n{\"binary\":false,\"payload\":\"y\",\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"get-stdin", []string{"get"}, "{\"key\":\"k-put-binary-ok\",\"binary\":true}\n{\"key\":\"2a\"}\n", nil,
			"\\{\"binary\":true,\"payload\":\"aGVsbG8y\",\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}\\n{\"binary\":false,\"payload\":\"c\",\"stat\":\\{\"version\":0,\"created_timestamp\":\\d+,\"modified_timestamp\":\\d+\\}\\}",
			"^$",
		},
		{"get-bad-binary-use", []string{"get", "-b"}, "", get.ErrorIncorrectBinaryFlagUse,
			".*",
			"Error: binary flag was set when config is being sourced from stdin",
		},
		{"list-none", []string{"list", "-n", "XXX", "-x", "XXY"}, "", nil,
			"\\{\"keys\":\\[\\]\\}",
			"^$",
		},
		{"list-all", []string{"list", "-n", "a", "-x", "z"}, "", nil,
			"\\{\"keys\":\\[\"k-put\",\"k-put-binary-ok\"\\]\\}",
			"^$",
		},
		{"list-no-minimum", []string{"list", "-x", "XXY"}, "", list.ErrorExpectedRangeInconsistent,
			".*",
			"Error: inconsistent flags; min and max flags must be in pairs",
		},
		{"list-no-maximum", []string{"list", "-n", "XXX"}, "", list.ErrorExpectedRangeInconsistent,
			".*",
			"Error: inconsistent flags; min and max flags must be in pairs",
		},
		{"list-stdin", []string{"list"}, "{\"key_minimum\":\"j\",\"key_maximum\":\"l\"}\n{\"key_minimum\":\"a\",\"key_maximum\":\"b\"}\n", nil,
			"\\{\"keys\":\\[\"k-put\",\"k-put-binary-ok\"\\]\\}",
			"^$",
		},
		{"delete", []string{"delete", "-k", "k-put-binary-ok"}, "", nil,
			"\\{\\}",
			"^$",
		},
		{"delete-not-exist", []string{"delete", "-k", "does-not-exist"}, "", nil,
			"\\{\"error\":\"key not found\"\\}",
			"^$",
		},
		{"delete-unexpected-version", []string{"delete", "-k", "k-put", "-e", "9"}, "", nil,
			"\\{\"error\":\"unexpected version\"\\}",
			"^$",
		},
		{"delete-expected-version", []string{"delete", "-k", "k-put", "-e", "1"}, "", nil,
			"\\{\\}",
			"^$",
		},
		{"delete-multi", []string{"delete", "-k", "2a", "-k", "2x"}, "", nil,
			"\\{\\}",
			"^$",
		},
		{"delete-multi-not-exist", []string{"delete", "-k", "2a", "-k", "2x"}, "", nil,
			"\\{\"error\":\"key not found\"\\}",
			"^$",
		},
		{"delete-multi-with-expected", []string{"delete", "-k", "2a", "-e", "0", "-k", "2x", "-e", "0"}, "", nil,
			"\\{\"error\":\"unexpected version\"\\}\n\\{\"error\":\"unexpected version\"\\}\n",
			"^$",
		},
		{"delete-range", []string{"delete", "-n", "q", "-x", "s"}, "", nil,
			"\\{\\}",
			"^$",
		},
		{"delete-range-with-expected", []string{"delete", "-n", "q", "-x", "s", "-e", "0"}, "", delete.ErrorExpectedVersionInconsistent,
			".*",
			"Error: inconsistent flags; zero or all keys must have an expected version",
		},
		{"delete-stdin", []string{"delete"}, "{\"key_minimum\":\"j\",\"key_maximum\":\"l\"}\n{\"key\":\"a\"}\n\n{\"key\":\"a\",\"expected_version\":0}\n", nil,
			"\\{\\}",
			"^$",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			put.Config.Reset()
			get.Config.Reset()
			list.Config.Reset()
			delete.Config.Reset()

			stdin.WriteString(test.stdin)
			Cmd.SetArgs(append([]string{"-a", serviceAddress}, test.args...))
			err := Cmd.Execute()

			assert.Regexp(t, test.expectedStdOutRe, stdout.String())
			assert.Regexp(t, test.expectedStdErrRe, stderr.String())
			assert.ErrorIs(t, err, test.expectedErr)

			stdin.Reset()
			stdout.Reset()
			stderr.Reset()
		})
	}
	server.Close()
}