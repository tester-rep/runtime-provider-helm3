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

package action

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"helm.sh/helm/pkg/chartutil"
	"helm.sh/helm/pkg/lint"
	"helm.sh/helm/pkg/lint/support"
)

var errLintNoChart = errors.New("no chart found for linting (missing Chart.yaml)")

// Lint is the action for checking that the semantics of a chart are well-formed.
//
// It provides the implementation of 'helm lint'.
type Lint struct {
	Strict    bool
	Namespace string
}

type LintResult struct {
	TotalChartsLinted int
	Messages          []support.Message
	Errors            []error
}

// NewLint creates a new Lint object with the given configuration.
func NewLint() *Lint {
	return &Lint{}
}

// Run executes 'helm Lint' against the given chart.
func (l *Lint) Run(paths []string, vals map[string]interface{}) *LintResult {
	lowestTolerance := support.ErrorSev
	if l.Strict {
		lowestTolerance = support.WarningSev
	}

	result := &LintResult{}
	for _, path := range paths {
		linter, err := lintChart(path, vals, l.Namespace, l.Strict)
		if err != nil {
			if err == errLintNoChart {
				result.Errors = append(result.Errors, err)
			}
			if linter.HighestSeverity >= lowestTolerance {
				result.Errors = append(result.Errors, err)
			}
		} else {
			result.Messages = append(result.Messages, linter.Messages...)
			result.TotalChartsLinted++
			for _, msg := range linter.Messages {
				if msg.Severity == support.ErrorSev {
					result.Errors = append(result.Errors, msg.Err)
				}
			}
		}
	}
	return result
}

func lintChart(path string, vals map[string]interface{}, namespace string, strict bool) (support.Linter, error) {
	var chartPath string
	linter := support.Linter{}
	currentVals := make(map[string]interface{}, len(vals))
	for key, value := range vals {
		currentVals[key] = value
	}

	if strings.HasSuffix(path, ".tgz") {
		tempDir, err := ioutil.TempDir("", "helm-lint")
		if err != nil {
			return linter, err
		}
		defer os.RemoveAll(tempDir)

		file, err := os.Open(path)
		if err != nil {
			return linter, err
		}
		defer file.Close()

		if err = chartutil.Expand(tempDir, file); err != nil {
			return linter, err
		}

		lastHyphenIndex := strings.LastIndex(filepath.Base(path), "-")
		if lastHyphenIndex <= 0 {
			return linter, errors.Errorf("unable to parse chart archive %q, missing '-'", filepath.Base(path))
		}
		base := filepath.Base(path)[:lastHyphenIndex]
		chartPath = filepath.Join(tempDir, base)
	} else {
		chartPath = path
	}

	// Guard: Error out of this is not a chart.
	if _, err := os.Stat(filepath.Join(chartPath, "Chart.yaml")); err != nil {
		return linter, errLintNoChart
	}

	return lint.All(chartPath, currentVals, namespace, strict), nil
}
