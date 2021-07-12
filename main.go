package helm_template_testing

import (
	"fmt"
	"github.com/gruntwork-io/terratest/modules/logger"
	"io"

	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terratest/modules/helm"
)

type Options struct {
	TestsDir string
	ChartDir string
}

type AKNN struct {
	ApiVersion string
	Kind string
	Name string
	Namespace string
}

func (a AKNN) String() string {
	s := fmt.Sprintf("%s.%s", a.ApiVersion, a.Kind)

	if a.Namespace != "" {
		s = fmt.Sprintf("%s/%s", s, a.Namespace)
	}

	return fmt.Sprintf("%s/%s", s, a.Name)
}

type Test struct {
	Name           string
	ValuesFilePath string
	Expectations   map[AKNN]interface{}
}

type TestSuite struct {
	Tests []Test
}

func TestChartTemplate(t *testing.T, chartDir string, testsDir string)  {
	opt := Options{
		TestsDir: testsDir,
		ChartDir: chartDir,
	}

	chartPath, err := filepath.Abs(opt.ChartDir)
	require.NoErrorf(t, err, "Failed to resolve chart directory path")
	opt.ChartDir = chartPath

	testPath, err := filepath.Abs(opt.TestsDir)
	require.NoErrorf(t, err, "Failed to resolve tests directory path")
	opt.TestsDir = testPath

	suite, err := createTestSuite(t, opt)
	require.NoErrorf(t, err, "Failed to build test suite")

	for _, test := range suite.Tests {
		t.Run(test.Name, func(t *testing.T) {
			releaseName := "helm-test"

			options := &helm.Options{
				ValuesFiles: []string{test.ValuesFilePath},
				Logger: logger.Discard,
			}

			outputYaml := helm.RenderTemplate(t, options, opt.ChartDir, releaseName, []string{})
			output := decodeYAMLResources(t, outputYaml)

			expectedResourceOccurence := make(map[AKNN]bool)
			for aknn := range test.Expectations {
				expectedResourceOccurence[aknn] = false
			}

			for aknn, resource := range output {
				expected, found := test.Expectations[aknn]
				if !found {
					t.Errorf("Resource %s not found on expected values", aknn)
					continue
				}

				expectedResourceOccurence[aknn] = true
				require.Equal(t, expected, resource)
			}

			for aknn, occured := range expectedResourceOccurence {
				if !occured {
					t.Errorf("Resource %s is expected but none was rendered", aknn)
				}
			}
		})
	}
}

func decodeYAMLResources(t *testing.T, outputYaml string) map[AKNN]interface{} {
	decoder := yaml.NewDecoder(strings.NewReader(outputYaml))

	output := make(map[AKNN]interface{})
	for {
		var resource map[string]interface{}
		err := decoder.Decode(&resource)
		if err == io.EOF {
			break
		}

		ann := makeANN(t, resource)

		require.NoError(t, err)
		output[ann] = resource
	}

	return output
}

func makeANN(t *testing.T, resource map[string]interface{}) AKNN {
	apiVersion, ok := resource["apiVersion"].(string)
	require.Truef(t, ok, "Failed to cast apiVersion field on resource: %#+v", resource)
	kind := resource["kind"].(string)
	require.Truef(t, ok, "Failed to cast kind field on resource: %#+v", resource)

	metadata, ok := resource["metadata"].(map[string]interface{})
	require.Truef(t, ok, "Failed to cast metadata field on resource: %#+v", resource)

	name, ok := metadata["name"].(string)
	require.Truef(t, ok, "Failed to cast name field on resource: %#+v", resource)

	akkn := AKNN{
		ApiVersion: apiVersion,
		Kind:       kind,
		Name:       name,
	}

	namespace, found := metadata["namespace"]
	if found {
		namespaceStr, ok := namespace.(string)
		require.Truef(t, ok, "Failed to cast namespace field on resource: %#+v", resource)
		akkn.Namespace = namespaceStr
	}

	return akkn
}

func createTestSuite(t *testing.T, opt Options) (TestSuite, error) {
	dirFS := os.DirFS(opt.TestsDir)
	rootDir := "."

	suiteIndex := map[string]Test{}

	err := fs.WalkDir(dirFS, rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == rootDir {
			return nil
		}

		if d.IsDir() {
			suiteIndex[path] = Test{Name: path, Expectations: make(map[AKNN]interface{})}
			return nil
		}

		testDir := filepath.Dir(path)
		filename := filepath.Base(path)

		test, ok := suiteIndex[testDir]
		if !ok {
			test = Test{Name: testDir, Expectations: make(map[AKNN]interface{})}
		}

		if filename == "values.yml" {
			absPath, err := filepath.Abs(filepath.Join(opt.TestsDir, path))
			require.NoError(t, err)

			test.ValuesFilePath = absPath
		}

		if strings.HasPrefix(filename, "expected_") {
			absPath, err := filepath.Abs(filepath.Join(opt.TestsDir, path))
			require.NoError(t, err)

			file, err := ioutil.ReadFile(absPath)
			require.NoErrorf(t, err, "Failed to read expectation file")

			var resource map[string]interface{}
			err = yaml.Unmarshal(file, &resource)
			require.NoError(t, err)

			aknn := makeANN(t, resource)

			test.Expectations[aknn] = resource
		}

		suiteIndex[testDir] = test

		return nil
	})

	require.NoError(t, err)

	var suite TestSuite
	for _, t := range suiteIndex {
		suite.Tests = append(suite.Tests, t)
	}

	return suite, nil
}
