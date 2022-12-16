package delete

import (
	"encoding/json"
	"errors"
	"github.com/spf13/cobra"
	"io"
	"oxia/cmd/client/common"
	"oxia/oxia"
)

var (
	Config = flags{}

	ErrorExpectedVersionInconsistent = errors.New("inconsistent flags; zero or all keys must have an expected version")
	ErrorExpectedRangeInconsistent   = errors.New("inconsistent flags; min and max flags must be in pairs")
)

type flags struct {
	keys             []string
	expectedVersions []int64
	keyMinimums      []string
	keyMaximums      []string
}

func (flags *flags) Reset() {
	flags.keys = nil
	flags.expectedVersions = nil
	flags.keyMinimums = nil
	flags.keyMaximums = nil
}

func init() {
	Cmd.Flags().StringSliceVarP(&Config.keys, "key", "k", []string{}, "The target key")
	Cmd.Flags().Int64SliceVarP(&Config.expectedVersions, "expected-version", "e", []int64{}, "Version of entry expected to be on the server")
	Cmd.Flags().StringSliceVarP(&Config.keyMinimums, "key-min", "n", []string{}, "Key range minimum (inclusive)")
	Cmd.Flags().StringSliceVarP(&Config.keyMaximums, "key-max", "x", []string{}, "Key range maximum (exclusive)")
	Cmd.MarkFlagsRequiredTogether("key-min", "key-max")
}

var Cmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete the entries",
	Long:  `Delete the entries with the given keys or key ranges, if they exists. If an expected version is provided, the delete will only take place if it matches the version of the current record on the server`,
	Args:  cobra.NoArgs,
	RunE:  exec,
}

func exec(cmd *cobra.Command, args []string) error {
	loop, _ := common.NewCommandLoop(cmd.OutOrStdout())
	defer func() {
		loop.Complete()
	}()
	return _exec(Config, cmd.InOrStdin(), loop)
}

func _exec(flags flags, in io.Reader, queue common.QueryQueue) error {
	if len(flags.keyMinimums) != len(flags.keyMaximums) {
		return ErrorExpectedRangeInconsistent
	}
	if len(flags.keys) != len(flags.expectedVersions) && len(flags.expectedVersions) > 0 {
		return ErrorExpectedVersionInconsistent
	}
	if len(flags.keyMinimums) > 0 || len(flags.keys) > 0 {
		for i, n := range flags.keyMinimums {
			queue.Add(QueryByRange{
				KeyMinimum: n,
				KeyMaximum: flags.keyMaximums[i],
			})
		}
		for i, k := range flags.keys {
			query := QueryByKey{
				Key: k,
			}
			if len(flags.expectedVersions) > 0 {
				query.ExpectedVersion = &flags.expectedVersions[i]
			}
			queue.Add(query)
		}
	} else {
		common.ReadStdin(in, QueryInput{}, queue)
	}
	return nil
}

type QueryInput struct {
	Key             *string `json:"key,omitempty"`
	ExpectedVersion *int64  `json:"expected_version,omitempty"`
	KeyMinimum      *string `json:"key_minimum,omitempty"`
	KeyMaximum      *string `json:"key_maximum,omitempty"`
}

func (query QueryInput) Unmarshal(b []byte) (common.Query, error) {
	q := QueryInput{}
	err := json.Unmarshal(b, &q)
	if q.Key == nil {
		return QueryByRange{
			KeyMinimum: *q.KeyMinimum,
			KeyMaximum: *q.KeyMaximum,
		}, err
	} else {
		return QueryByKey{
			Key:             *q.Key,
			ExpectedVersion: q.ExpectedVersion,
		}, err
	}
}

type QueryByKey struct {
	Key             string
	ExpectedVersion *int64
}

func (query QueryByKey) Perform(client oxia.AsyncClient) common.Call {
	return Call{
		clientCall: client.Delete(query.Key, query.ExpectedVersion),
	}
}

type QueryByRange struct {
	KeyMinimum string
	KeyMaximum string
}

func (query QueryByRange) Perform(client oxia.AsyncClient) common.Call {
	return Call{
		clientCall: client.DeleteRange(query.KeyMinimum, query.KeyMaximum),
	}
}

type Call struct {
	clientCall <-chan error
}

func (call Call) Complete() any {
	result := <-call.clientCall
	output := common.OutputError{}
	if result != nil {
		output.Err = result.Error()
	}
	return output
}