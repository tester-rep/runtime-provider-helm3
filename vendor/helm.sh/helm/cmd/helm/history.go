/*
Copyright The Helm Authors.

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

package main

import (
	"fmt"
	"io"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"

	"helm.sh/helm/cmd/helm/require"
	"helm.sh/helm/pkg/action"
	"helm.sh/helm/pkg/chart"
	"helm.sh/helm/pkg/release"
	"helm.sh/helm/pkg/releaseutil"
)

var historyHelp = `
History prints historical revisions for a given release.

A default maximum of 256 revisions will be returned. Setting '--max'
configures the maximum length of the revision list returned.

The historical release set is printed as a formatted table, e.g:

    $ helm history angry-bird --max=4
    REVISION    UPDATED                     STATUS          CHART             APP VERSION     DESCRIPTION
    1           Mon Oct 3 10:15:13 2016     superseded      alpine-0.1.0      1.0             Initial install
    2           Mon Oct 3 10:15:13 2016     superseded      alpine-0.1.0      1.0             Upgraded successfully
    3           Mon Oct 3 10:15:13 2016     superseded      alpine-0.1.0      1.0             Rolled back to 2
    4           Mon Oct 3 10:15:13 2016     deployed        alpine-0.1.0      1.0             Upgraded successfully
`

func newHistoryCmd(cfg *action.Configuration, out io.Writer) *cobra.Command {
	client := action.NewHistory(cfg)

	cmd := &cobra.Command{
		Use:     "history RELEASE_NAME",
		Long:    historyHelp,
		Short:   "fetch release history",
		Aliases: []string{"hist"},
		Args:    require.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			history, err := getHistory(client, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(out, history)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVarP(&client.OutputFormat, "output", "o", action.Table.String(), "prints the output in the specified format (json|table|yaml)")
	f.IntVar(&client.Max, "max", 256, "maximum number of revision to include in history")

	return cmd
}

type releaseInfo struct {
	Revision    int    `json:"revision"`
	Updated     string `json:"updated"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

type releaseHistory []releaseInfo

func marshalHistory(format action.OutputFormat, hist releaseHistory) (byt []byte, err error) {
	switch format {
	case action.YAML, action.JSON:
		byt, err = format.Marshal(hist)
	case action.Table:
		byt, err = format.MarshalTable(func(tbl *uitable.Table) {
			tbl.AddRow("REVISION", "UPDATED", "STATUS", "CHART", "APP VERSION", "DESCRIPTION")
			for i := 0; i <= len(hist)-1; i++ {
				r := hist[i]
				tbl.AddRow(r.Revision, r.Updated, r.Status, r.Chart, r.AppVersion, r.Description)
			}
		})
	default:
		err = action.ErrInvalidFormatType
	}
	return
}

func getHistory(client *action.History, name string) (string, error) {
	hist, err := client.Run(name)
	if err != nil {
		return "", err
	}

	releaseutil.Reverse(hist, releaseutil.SortByRevision)

	var rels []*release.Release
	for i := 0; i < min(len(hist), client.Max); i++ {
		rels = append(rels, hist[i])
	}

	if len(rels) == 0 {
		return "", nil
	}

	releaseHistory := getReleaseHistory(rels)

	outputFormat, err := action.ParseOutputFormat(client.OutputFormat)
	if err != nil {
		return "", err
	}
	history, formattingError := marshalHistory(outputFormat, releaseHistory)
	if formattingError != nil {
		return "", formattingError
	}

	return string(history), nil
}

func getReleaseHistory(rls []*release.Release) (history releaseHistory) {
	for i := len(rls) - 1; i >= 0; i-- {
		r := rls[i]
		c := formatChartname(r.Chart)
		s := r.Info.Status.String()
		v := r.Version
		d := r.Info.Description
		a := formatAppVersion(r.Chart)

		rInfo := releaseInfo{
			Revision:    v,
			Status:      s,
			Chart:       c,
			AppVersion:  a,
			Description: d,
		}
		if !r.Info.LastDeployed.IsZero() {
			rInfo.Updated = r.Info.LastDeployed.String()

		}
		history = append(history, rInfo)
	}

	return history
}

func formatChartname(c *chart.Chart) string {
	if c == nil || c.Metadata == nil {
		// This is an edge case that has happened in prod, though we don't
		// know how: https://github.com/helm/helm/issues/1347
		return "MISSING"
	}
	return fmt.Sprintf("%s-%s", c.Name(), c.Metadata.Version)
}

func formatAppVersion(c *chart.Chart) string {
	if c == nil || c.Metadata == nil {
		// This is an edge case that has happened in prod, though we don't
		// know how: https://github.com/helm/helm/issues/1347
		return "MISSING"
	}
	return c.AppVersion()
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
