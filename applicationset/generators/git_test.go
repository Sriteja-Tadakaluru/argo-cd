package generators

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/argoproj/argo-cd/v3/applicationset/services/mocks"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

func Test_generateParamsFromGitFile(t *testing.T) {
	defaultContent := []byte(`
foo:
  bar: baz
`)
	type args struct {
		filePath          string
		fileContent       []byte
		values            map[string]string
		useGoTemplate     bool
		goTemplateOptions []string
		pathParamPrefix   string
	}
	tests := []struct {
		name    string
		args    args
		want    []map[string]any
		wantErr bool
	}{
		{
			name: "empty file returns path parameters",
			args: args{
				filePath:      "path/dir/file_name.yaml",
				fileContent:   []byte(""),
				values:        map[string]string{},
				useGoTemplate: false,
			},
			want: []map[string]any{
				{
					"path":                    "path/dir",
					"path.basename":           "dir",
					"path.filename":           "file_name.yaml",
					"path.basenameNormalized": "dir",
					"path.filenameNormalized": "file-name.yaml",
					"path[0]":                 "path",
					"path[1]":                 "dir",
				},
			},
		},
		{
			name: "invalid json/yaml file returns error",
			args: args{
				filePath:      "path/dir/file_name.yaml",
				fileContent:   []byte("this is not json or yaml"),
				values:        map[string]string{},
				useGoTemplate: false,
			},
			wantErr: true,
		},
		{
			name: "file parameters are added to params",
			args: args{
				filePath:      "path/dir/file_name.yaml",
				fileContent:   defaultContent,
				values:        map[string]string{},
				useGoTemplate: false,
			},
			want: []map[string]any{
				{
					"foo.bar":                 "baz",
					"path":                    "path/dir",
					"path.basename":           "dir",
					"path.filename":           "file_name.yaml",
					"path.basenameNormalized": "dir",
					"path.filenameNormalized": "file-name.yaml",
					"path[0]":                 "path",
					"path[1]":                 "dir",
				},
			},
		},
		{
			name: "path parameter are prefixed",
			args: args{
				filePath:        "path/dir/file_name.yaml",
				fileContent:     defaultContent,
				values:          map[string]string{},
				useGoTemplate:   false,
				pathParamPrefix: "myRepo",
			},
			want: []map[string]any{
				{
					"foo.bar":                        "baz",
					"myRepo.path":                    "path/dir",
					"myRepo.path.basename":           "dir",
					"myRepo.path.filename":           "file_name.yaml",
					"myRepo.path.basenameNormalized": "dir",
					"myRepo.path.filenameNormalized": "file-name.yaml",
					"myRepo.path[0]":                 "path",
					"myRepo.path[1]":                 "dir",
				},
			},
		},
		{
			name: "file parameters are added to params with go template",
			args: args{
				filePath:      "path/dir/file_name.yaml",
				fileContent:   defaultContent,
				values:        map[string]string{},
				useGoTemplate: true,
			},
			want: []map[string]any{
				{
					"foo": map[string]any{
						"bar": "baz",
					},
					"path": map[string]any{
						"path":               "path/dir",
						"basename":           "dir",
						"filename":           "file_name.yaml",
						"basenameNormalized": "dir",
						"filenameNormalized": "file-name.yaml",
						"segments": []string{
							"path",
							"dir",
						},
					},
				},
			},
		},
		{
			name: "path parameter are prefixed with go template",
			args: args{
				filePath:        "path/dir/file_name.yaml",
				fileContent:     defaultContent,
				values:          map[string]string{},
				useGoTemplate:   true,
				pathParamPrefix: "myRepo",
			},
			want: []map[string]any{
				{
					"foo": map[string]any{
						"bar": "baz",
					},
					"myRepo": map[string]any{
						"path": map[string]any{
							"path":               "path/dir",
							"basename":           "dir",
							"filename":           "file_name.yaml",
							"basenameNormalized": "dir",
							"filenameNormalized": "file-name.yaml",
							"segments": []string{
								"path",
								"dir",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := (*GitGenerator)(nil).generateParamsFromGitFile(tt.args.filePath, tt.args.fileContent, tt.args.values, tt.args.useGoTemplate, tt.args.goTemplateOptions, tt.args.pathParamPrefix)
			if tt.wantErr {
				assert.Error(t, err, "GitGenerator.generateParamsFromGitFile()")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, params)
			}
		})
	}
}

func TestGitGenerateParamsFromDirectories(t *testing.T) {
	cases := []struct {
		name            string
		directories     []v1alpha1.GitDirectoryGeneratorItem
		pathParamPrefix string
		repoApps        []string
		repoError       error
		values          map[string]string
		expected        []map[string]any
		expectedError   error
	}{
		{
			name:        "happy flow - created apps",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			repoApps: []string{
				"app1",
				"app2",
				"app_3",
				"p1/app4",
			},
			expected: []map[string]any{
				{"path": "app1", "path.basename": "app1", "path.basenameNormalized": "app1", "path[0]": "app1"},
				{"path": "app2", "path.basename": "app2", "path.basenameNormalized": "app2", "path[0]": "app2"},
				{"path": "app_3", "path.basename": "app_3", "path.basenameNormalized": "app-3", "path[0]": "app_3"},
			},
			expectedError: nil,
		},
		{
			name:            "It prefixes path parameters with PathParamPrefix",
			directories:     []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			pathParamPrefix: "myRepo",
			repoApps: []string{
				"app1",
				"app2",
				"app_3",
				"p1/app4",
			},
			repoError: nil,
			expected: []map[string]any{
				{"myRepo.path": "app1", "myRepo.path.basename": "app1", "myRepo.path.basenameNormalized": "app1", "myRepo.path[0]": "app1"},
				{"myRepo.path": "app2", "myRepo.path.basename": "app2", "myRepo.path.basenameNormalized": "app2", "myRepo.path[0]": "app2"},
				{"myRepo.path": "app_3", "myRepo.path.basename": "app_3", "myRepo.path.basenameNormalized": "app-3", "myRepo.path[0]": "app_3"},
			},
			expectedError: nil,
		},
		{
			name:        "It filters application according to the paths",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "p1/*"}, {Path: "p1/*/*"}},
			repoApps: []string{
				"app1",
				"p1/app2",
				"p1/p2/app3",
				"p1/p2/p3/app4",
			},
			expected: []map[string]any{
				{"path": "p1/app2", "path.basename": "app2", "path[0]": "p1", "path[1]": "app2", "path.basenameNormalized": "app2"},
				{"path": "p1/p2/app3", "path.basename": "app3", "path[0]": "p1", "path[1]": "p2", "path[2]": "app3", "path.basenameNormalized": "app3"},
			},
			expectedError: nil,
		},
		{
			name:        "It filters application according to the paths with Exclude",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "p1/*", Exclude: true}, {Path: "*"}, {Path: "*/*"}},
			repoApps: []string{
				"app1",
				"app2",
				"p1/app2",
				"p1/app3",
				"p2/app3",
			},
			repoError: nil,
			expected: []map[string]any{
				{"path": "app1", "path.basename": "app1", "path[0]": "app1", "path.basenameNormalized": "app1"},
				{"path": "app2", "path.basename": "app2", "path[0]": "app2", "path.basenameNormalized": "app2"},
				{"path": "p2/app3", "path.basename": "app3", "path[0]": "p2", "path[1]": "app3", "path.basenameNormalized": "app3"},
			},
			expectedError: nil,
		},
		{
			name:        "Expecting same exclude behavior with different order",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}, {Path: "*/*"}, {Path: "p1/*", Exclude: true}},
			repoApps: []string{
				"app1",
				"app2",
				"p1/app2",
				"p1/app3",
				"p2/app3",
			},
			repoError: nil,
			expected: []map[string]any{
				{"path": "app1", "path.basename": "app1", "path[0]": "app1", "path.basenameNormalized": "app1"},
				{"path": "app2", "path.basename": "app2", "path[0]": "app2", "path.basenameNormalized": "app2"},
				{"path": "p2/app3", "path.basename": "app3", "path[0]": "p2", "path[1]": "app3", "path.basenameNormalized": "app3"},
			},
			expectedError: nil,
		},
		{
			name:        "Value variable interpolation",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}, {Path: "*/*"}},
			repoApps: []string{
				"app1",
				"p1/app2",
			},
			repoError: nil,
			values: map[string]string{
				"foo":   "bar",
				"aaa":   "{{ path[0] }}",
				"no-op": "{{ this-does-not-exist }}",
			},
			expected: []map[string]any{
				{"values.foo": "bar", "values.no-op": "{{ this-does-not-exist }}", "values.aaa": "app1", "path": "app1", "path.basename": "app1", "path[0]": "app1", "path.basenameNormalized": "app1"},
				{"values.foo": "bar", "values.no-op": "{{ this-does-not-exist }}", "values.aaa": "p1", "path": "p1/app2", "path.basename": "app2", "path[0]": "p1", "path[1]": "app2", "path.basenameNormalized": "app2"},
			},
			expectedError: nil,
		},
		{
			name:          "handles empty response from repo server",
			directories:   []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			repoApps:      []string{},
			repoError:     nil,
			expected:      []map[string]any{},
			expectedError: nil,
		},
		{
			name:          "handles error from repo server",
			directories:   []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			repoApps:      []string{},
			repoError:     errors.New("error"),
			expected:      []map[string]any{},
			expectedError: errors.New("error generating params from git: error getting directories from repo: error"),
		},
	}

	for _, testCase := range cases {
		testCaseCopy := testCase

		t.Run(testCaseCopy.name, func(t *testing.T) {
			t.Parallel()

			argoCDServiceMock := mocks.Repos{}

			argoCDServiceMock.On("GetDirectories", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(testCaseCopy.repoApps, testCaseCopy.repoError)

			gitGenerator := NewGitGenerator(&argoCDServiceMock, "")
			applicationSetInfo := v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "set",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:         "RepoURL",
							Revision:        "Revision",
							Directories:     testCaseCopy.directories,
							PathParamPrefix: testCaseCopy.pathParamPrefix,
							Values:          testCaseCopy.values,
						},
					}},
				},
			}

			scheme := runtime.NewScheme()
			err := v1alpha1.AddToScheme(scheme)
			require.NoError(t, err)
			appProject := v1alpha1.AppProject{}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&appProject).Build()

			got, err := gitGenerator.GenerateParams(&applicationSetInfo.Spec.Generators[0], &applicationSetInfo, client)

			if testCaseCopy.expectedError != nil {
				require.EqualError(t, err, testCaseCopy.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, testCaseCopy.expected, got)
			}

			argoCDServiceMock.AssertExpectations(t)
		})
	}
}

func TestGitGenerateParamsFromDirectoriesGoTemplate(t *testing.T) {
	cases := []struct {
		name            string
		directories     []v1alpha1.GitDirectoryGeneratorItem
		pathParamPrefix string
		repoApps        []string
		repoError       error
		expected        []map[string]any
		expectedError   error
	}{
		{
			name:        "happy flow - created apps",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			repoApps: []string{
				"app1",
				"app2",
				"app_3",
				"p1/app4",
			},
			repoError: nil,
			expected: []map[string]any{
				{
					"path": map[string]any{
						"path":               "app1",
						"basename":           "app1",
						"basenameNormalized": "app1",
						"segments": []string{
							"app1",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "app2",
						"basename":           "app2",
						"basenameNormalized": "app2",
						"segments": []string{
							"app2",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "app_3",
						"basename":           "app_3",
						"basenameNormalized": "app-3",
						"segments": []string{
							"app_3",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:            "It prefixes path parameters with PathParamPrefix",
			directories:     []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			pathParamPrefix: "myRepo",
			repoApps: []string{
				"app1",
				"app2",
				"app_3",
				"p1/app4",
			},
			repoError: nil,
			expected: []map[string]any{
				{
					"myRepo": map[string]any{
						"path": map[string]any{
							"path":               "app1",
							"basename":           "app1",
							"basenameNormalized": "app1",
							"segments": []string{
								"app1",
							},
						},
					},
				},
				{
					"myRepo": map[string]any{
						"path": map[string]any{
							"path":               "app2",
							"basename":           "app2",
							"basenameNormalized": "app2",
							"segments": []string{
								"app2",
							},
						},
					},
				},
				{
					"myRepo": map[string]any{
						"path": map[string]any{
							"path":               "app_3",
							"basename":           "app_3",
							"basenameNormalized": "app-3",
							"segments": []string{
								"app_3",
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:        "It filters application according to the paths",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "p1/*"}, {Path: "p1/*/*"}},
			repoApps: []string{
				"app1",
				"p1/app2",
				"p1/p2/app3",
				"p1/p2/p3/app4",
			},
			repoError: nil,
			expected: []map[string]any{
				{
					"path": map[string]any{
						"path":               "p1/app2",
						"basename":           "app2",
						"basenameNormalized": "app2",
						"segments": []string{
							"p1",
							"app2",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "p1/p2/app3",
						"basename":           "app3",
						"basenameNormalized": "app3",
						"segments": []string{
							"p1",
							"p2",
							"app3",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:        "It filters application according to the paths with Exclude",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "p1/*", Exclude: true}, {Path: "*"}, {Path: "*/*"}},
			repoApps: []string{
				"app1",
				"app2",
				"p1/app2",
				"p1/app3",
				"p2/app3",
			},
			repoError: nil,
			expected: []map[string]any{
				{
					"path": map[string]any{
						"path":               "app1",
						"basename":           "app1",
						"basenameNormalized": "app1",
						"segments": []string{
							"app1",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "app2",
						"basename":           "app2",
						"basenameNormalized": "app2",
						"segments": []string{
							"app2",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "p2/app3",
						"basename":           "app3",
						"basenameNormalized": "app3",
						"segments": []string{
							"p2",
							"app3",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:        "Expecting same exclude behavior with different order",
			directories: []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}, {Path: "*/*"}, {Path: "p1/*", Exclude: true}},
			repoApps: []string{
				"app1",
				"app2",
				"p1/app2",
				"p1/app3",
				"p2/app3",
			},
			repoError: nil,
			expected: []map[string]any{
				{
					"path": map[string]any{
						"path":               "app1",
						"basename":           "app1",
						"basenameNormalized": "app1",
						"segments": []string{
							"app1",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "app2",
						"basename":           "app2",
						"basenameNormalized": "app2",
						"segments": []string{
							"app2",
						},
					},
				},
				{
					"path": map[string]any{
						"path":               "p2/app3",
						"basename":           "app3",
						"basenameNormalized": "app3",
						"segments": []string{
							"p2",
							"app3",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:          "handles empty response from repo server",
			directories:   []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			repoApps:      []string{},
			repoError:     nil,
			expected:      []map[string]any{},
			expectedError: nil,
		},
		{
			name:          "handles error from repo server",
			directories:   []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
			repoApps:      []string{},
			repoError:     errors.New("error"),
			expected:      []map[string]any{},
			expectedError: errors.New("error generating params from git: error getting directories from repo: error"),
		},
	}

	for _, testCase := range cases {
		testCaseCopy := testCase

		t.Run(testCaseCopy.name, func(t *testing.T) {
			t.Parallel()

			argoCDServiceMock := mocks.Repos{}

			argoCDServiceMock.On("GetDirectories", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(testCaseCopy.repoApps, testCaseCopy.repoError)

			gitGenerator := NewGitGenerator(&argoCDServiceMock, "")
			applicationSetInfo := v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "set",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					GoTemplate: true,
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:         "RepoURL",
							Revision:        "Revision",
							Directories:     testCaseCopy.directories,
							PathParamPrefix: testCaseCopy.pathParamPrefix,
						},
					}},
				},
			}

			scheme := runtime.NewScheme()
			err := v1alpha1.AddToScheme(scheme)
			require.NoError(t, err)
			appProject := v1alpha1.AppProject{}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&appProject).Build()

			got, err := gitGenerator.GenerateParams(&applicationSetInfo.Spec.Generators[0], &applicationSetInfo, client)

			if testCaseCopy.expectedError != nil {
				require.EqualError(t, err, testCaseCopy.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, testCaseCopy.expected, got)
			}

			argoCDServiceMock.AssertExpectations(t)
		})
	}
}

func TestGitGenerateParamsFromFiles(t *testing.T) {
	cases := []struct {
		name string
		// files is the list of paths/globs to match
		files []v1alpha1.GitFileGeneratorItem
		// repoFileContents maps repo path to the literal contents of that path
		repoFileContents map[string][]byte
		// if repoPathsError is non-nil, the call to GetPaths(...) will return this error value
		repoPathsError error
		values         map[string]string
		expected       []map[string]any
		expectedError  error
	}{
		{
			name:  "happy flow: create params from git files",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
   "cluster": {
       "owner": "john.doe@example.com",
       "name": "production",
       "address": "https://kubernetes.default.svc"
   },
   "key1": "val1",
   "key2": {
       "key2_1": "val2_1",
       "key2_2": {
           "key2_2_1": "val2_2_1"
       }
   },
   "key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
   "cluster": {
       "owner": "foo.bar@example.com",
       "name": "staging",
       "address": "https://kubernetes.default.svc"
   }
}`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"key3":                    "123",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
				{
					"cluster.owner":           "foo.bar@example.com",
					"cluster.name":            "staging",
					"cluster.address":         "https://kubernetes.default.svc",
					"path":                    "cluster-config/staging",
					"path.basename":           "staging",
					"path[0]":                 "cluster-config",
					"path[1]":                 "staging",
					"path.basenameNormalized": "staging",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
			},
			expectedError: nil,
		},
		{
			name:  "Value variable interpolation",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
   "cluster": {
       "owner": "john.doe@example.com",
       "name": "production",
       "address": "https://kubernetes.default.svc"
   },
   "key1": "val1",
   "key2": {
       "key2_1": "val2_1",
       "key2_2": {
           "key2_2_1": "val2_2_1"
       }
   },
   "key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
   "cluster": {
       "owner": "foo.bar@example.com",
       "name": "staging",
       "address": "https://kubernetes.default.svc"
   }
}`),
			},
			repoPathsError: nil,
			values: map[string]string{
				"aaa":   "{{ cluster.owner }}",
				"no-op": "{{ this-does-not-exist }}",
			},
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"key3":                    "123",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
					"values.aaa":              "john.doe@example.com",
					"values.no-op":            "{{ this-does-not-exist }}",
				},
				{
					"cluster.owner":           "foo.bar@example.com",
					"cluster.name":            "staging",
					"cluster.address":         "https://kubernetes.default.svc",
					"path":                    "cluster-config/staging",
					"path.basename":           "staging",
					"path[0]":                 "cluster-config",
					"path[1]":                 "staging",
					"path.basenameNormalized": "staging",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
					"values.aaa":              "foo.bar@example.com",
					"values.no-op":            "{{ this-does-not-exist }}",
				},
			},
			expectedError: nil,
		},
		{
			name:             "handles error during getting repo paths",
			files:            []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{},
			repoPathsError:   errors.New("paths error"),
			expected:         []map[string]any{},
			expectedError:    errors.New("error generating params from git: paths error"),
		},
		{
			name:  "test invalid JSON file returns error",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`invalid json file`),
			},
			repoPathsError: nil,
			expected:       []map[string]any{},
			expectedError:  errors.New("error generating params from git: unable to process file 'cluster-config/production/config.json': unable to parse file: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type map[string]interface {}"),
		},
		{
			name:  "test JSON array",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`
[
	{
		"cluster": {
			"owner": "john.doe@example.com",
			"name": "production",
			"address": "https://kubernetes.default.svc",
			"inner": {
				"one" : "two"
			}
		}
	},
	{
		"cluster": {
			"owner": "john.doe@example.com",
			"name": "staging",
			"address": "https://kubernetes.default.svc"
		}
	}
]`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"cluster.inner.one":       "two",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "staging",
					"cluster.address":         "https://kubernetes.default.svc",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
			},
			expectedError: nil,
		},
		{
			name:  "Test YAML flow",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.yaml"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.yaml": []byte(`
cluster:
  owner: john.doe@example.com
  name: production
  address: https://kubernetes.default.svc
key1: val1
key2:
  key2_1: val2_1
  key2_2:
    key2_2_1: val2_2_1
`),
				"cluster-config/staging/config.yaml": []byte(`
cluster:
  owner: foo.bar@example.com
  name: staging
  address: https://kubernetes.default.svc
`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.yaml",
					"path.filenameNormalized": "config.yaml",
				},
				{
					"cluster.owner":           "foo.bar@example.com",
					"cluster.name":            "staging",
					"cluster.address":         "https://kubernetes.default.svc",
					"path":                    "cluster-config/staging",
					"path.basename":           "staging",
					"path[0]":                 "cluster-config",
					"path[1]":                 "staging",
					"path.basenameNormalized": "staging",
					"path.filename":           "config.yaml",
					"path.filenameNormalized": "config.yaml",
				},
			},
			expectedError: nil,
		},
		{
			name:  "test YAML array",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.yaml"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.yaml": []byte(`
- cluster:
    owner: john.doe@example.com
    name: production
    address: https://kubernetes.default.svc
    inner:
      one: two
- cluster:
    owner: john.doe@example.com
    name: staging
    address: https://kubernetes.default.svc`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"cluster.inner.one":       "two",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.yaml",
					"path.filenameNormalized": "config.yaml",
				},
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "staging",
					"cluster.address":         "https://kubernetes.default.svc",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.yaml",
					"path.filenameNormalized": "config.yaml",
				},
			},
			expectedError: nil,
		},
		{
			name: "create params from git files using matchLabels",
			files: []v1alpha1.GitFileGeneratorItem{{
				Path: "**/config.json",
				FileLabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"cluster.name": "production",
						"key1":         "val1",
					},
				},
			}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
"cluster": {
   "owner": "john.doe@example.com",
   "name": "production",
   "address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
"cluster": {
	"owner": "foo.bar@example.com",
	"name": "staging",
	"address": "https://kubernetes.default.svc"
}
}`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"key3":                    "123",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
			},
			expectedError: nil,
		},
		{
			name: "create params from git files using matchExpressions",
			files: []v1alpha1.GitFileGeneratorItem{{
				Path: "**/config.json",
				FileLabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "cluster.name", Operator: metav1.LabelSelectorOpIn, Values: []string{"production", "test"}},
					},
				},
			}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
"cluster": {
   "owner": "john.doe@example.com",
   "name": "production",
   "address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
"cluster": {
	"owner": "foo.bar@example.com",
	"name": "staging",
	"address": "https://kubernetes.default.svc"
}
}`),
				"cluster-config/test/config.json": []byte(`{
"cluster": {
	"owner": "clairk.gen@example.com",
	"name": "test",
	"address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123,
"key4": "val4",
	 }`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "john.doe@example.com",
					"cluster.name":            "production",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"key3":                    "123",
					"path":                    "cluster-config/production",
					"path.basename":           "production",
					"path[0]":                 "cluster-config",
					"path[1]":                 "production",
					"path.basenameNormalized": "production",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
				{
					"cluster.owner":           "clairk.gen@example.com",
					"cluster.name":            "test",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"key3":                    "123",
					"key4":                    "val4",
					"path":                    "cluster-config/test",
					"path.basename":           "test",
					"path[0]":                 "cluster-config",
					"path[1]":                 "test",
					"path.basenameNormalized": "test",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
			},
			expectedError: nil,
		},
		{
			name: "create params from git files matchLabels and matchExpressions",
			files: []v1alpha1.GitFileGeneratorItem{{
				Path: "**/config.json",
				FileLabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "cluster.name", Operator: metav1.LabelSelectorOpIn, Values: []string{"production", "test"}},
					},
					MatchLabels: map[string]string{
						"key4": "val4",
					},
				},
			}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
"cluster": {
   "owner": "john.doe@example.com",
   "name": "production",
   "address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
"cluster": {
	"owner": "foo.bar@example.com",
	"name": "staging",
	"address": "https://kubernetes.default.svc"
}
}`),
				"cluster-config/test/config.json": []byte(`{
"cluster": {
	"owner": "clairk.gen@example.com",
	"name": "test",
	"address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123,
"key4": "val4",
	 }`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster.owner":           "clairk.gen@example.com",
					"cluster.name":            "test",
					"cluster.address":         "https://kubernetes.default.svc",
					"key1":                    "val1",
					"key2.key2_1":             "val2_1",
					"key2.key2_2.key2_2_1":    "val2_2_1",
					"key3":                    "123",
					"key4":                    "val4",
					"path":                    "cluster-config/test",
					"path.basename":           "test",
					"path[0]":                 "cluster-config",
					"path[1]":                 "test",
					"path.basenameNormalized": "test",
					"path.filename":           "config.json",
					"path.filenameNormalized": "config.json",
				},
			},
			expectedError: nil,
		},
		{
			name: "invalid value for matchLabels",
			files: []v1alpha1.GitFileGeneratorItem{{
				Path: "**/config.json",
				FileLabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"cluster.owner": "john.doe@example.com",
					},
				},
			}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
"cluster": {
   "owner": "john.doe@example.com",
   "name": "production",
   "address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
"cluster": {
	"owner": "foo.bar@example.com",
	"name": "staging",
	"address": "https://kubernetes.default.svc"
}
}`),
			},
			repoPathsError: nil,
			expected:       []map[string]any{},
			expectedError:  errors.New("error generating params from git: error parsing the label selector: values[0][cluster.owner]: Invalid value: \"john.doe@example.com\": a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')"),
		},
		{
			name: "test no match found",
			files: []v1alpha1.GitFileGeneratorItem{{
				Path: "**/config.json",
				FileLabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"cluster.owner": "test-owner",
					},
				},
			}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
"cluster": {
   "owner": "john.doe@example.com",
   "name": "production",
   "address": "https://kubernetes.default.svc"
},
"key1": "val1",
"key2": {
   "key2_1": "val2_1",
   "key2_2": {
	   "key2_2_1": "val2_2_1"
   }
},
"key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
"cluster": {
	"owner": "foo.bar@example.com",
	"name": "staging",
	"address": "https://kubernetes.default.svc"
}
}`),
			},
			repoPathsError: nil,
			expected:       []map[string]any{},
			expectedError:  nil,
		},
	}

	for _, testCase := range cases {
		testCaseCopy := testCase

		t.Run(testCaseCopy.name, func(t *testing.T) {
			t.Parallel()

			argoCDServiceMock := mocks.Repos{}
			argoCDServiceMock.On("GetFiles", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(testCaseCopy.repoFileContents, testCaseCopy.repoPathsError)

			gitGenerator := NewGitGenerator(&argoCDServiceMock, "")
			applicationSetInfo := v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "set",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:  "RepoURL",
							Revision: "Revision",
							Files:    testCaseCopy.files,
							Values:   testCaseCopy.values,
						},
					}},
				},
			}

			scheme := runtime.NewScheme()
			err := v1alpha1.AddToScheme(scheme)
			require.NoError(t, err)
			appProject := v1alpha1.AppProject{}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&appProject).Build()

			got, err := gitGenerator.GenerateParams(&applicationSetInfo.Spec.Generators[0], &applicationSetInfo, client)
			fmt.Println(got, err)

			if testCaseCopy.expectedError != nil {
				require.EqualError(t, err, testCaseCopy.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, testCaseCopy.expected, got)
			}

			argoCDServiceMock.AssertExpectations(t)
		})
	}
}

func TestGitGenerateParamsFromFilesGoTemplate(t *testing.T) {
	cases := []struct {
		name string
		// files is the list of paths/globs to match
		files []v1alpha1.GitFileGeneratorItem
		// repoFileContents maps repo path to the literal contents of that path
		repoFileContents map[string][]byte
		// if repoPathsError is non-nil, the call to GetPaths(...) will return this error value
		repoPathsError error
		expected       []map[string]any
		expectedError  error
	}{
		{
			name:  "happy flow: create params from git files",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`{
   "cluster": {
       "owner": "john.doe@example.com",
       "name": "production",
       "address": "https://kubernetes.default.svc"
   },
   "key1": "val1",
   "key2": {
       "key2_1": "val2_1",
       "key2_2": {
           "key2_2_1": "val2_2_1"
       }
   },
   "key3": 123
}`),
				"cluster-config/staging/config.json": []byte(`{
   "cluster": {
       "owner": "foo.bar@example.com",
       "name": "staging",
       "address": "https://kubernetes.default.svc"
   }
}`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster": map[string]any{
						"owner":   "john.doe@example.com",
						"name":    "production",
						"address": "https://kubernetes.default.svc",
					},
					"key1": "val1",
					"key2": map[string]any{
						"key2_1": "val2_1",
						"key2_2": map[string]any{
							"key2_2_1": "val2_2_1",
						},
					},
					"key3": float64(123),
					"path": map[string]any{
						"path":               "cluster-config/production",
						"basename":           "production",
						"filename":           "config.json",
						"basenameNormalized": "production",
						"filenameNormalized": "config.json",
						"segments": []string{
							"cluster-config",
							"production",
						},
					},
				},
				{
					"cluster": map[string]any{
						"owner":   "foo.bar@example.com",
						"name":    "staging",
						"address": "https://kubernetes.default.svc",
					},
					"path": map[string]any{
						"path":               "cluster-config/staging",
						"basename":           "staging",
						"filename":           "config.json",
						"basenameNormalized": "staging",
						"filenameNormalized": "config.json",
						"segments": []string{
							"cluster-config",
							"staging",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:             "handles error during getting repo paths",
			files:            []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{},
			repoPathsError:   errors.New("paths error"),
			expected:         []map[string]any{},
			expectedError:    errors.New("error generating params from git: paths error"),
		},
		{
			name:  "test invalid JSON file returns error",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`invalid json file`),
			},
			repoPathsError: nil,
			expected:       []map[string]any{},
			expectedError:  errors.New("error generating params from git: unable to process file 'cluster-config/production/config.json': unable to parse file: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type map[string]interface {}"),
		},
		{
			name:  "test JSON array",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.json"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.json": []byte(`
[
	{
		"cluster": {
			"owner": "john.doe@example.com",
			"name": "production",
			"address": "https://kubernetes.default.svc",
			"inner": {
				"one" : "two"
			}
		}
	},
	{
		"cluster": {
			"owner": "john.doe@example.com",
			"name": "staging",
			"address": "https://kubernetes.default.svc"
		}
	}
]`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster": map[string]any{
						"owner":   "john.doe@example.com",
						"name":    "production",
						"address": "https://kubernetes.default.svc",
						"inner": map[string]any{
							"one": "two",
						},
					},
					"path": map[string]any{
						"path":               "cluster-config/production",
						"basename":           "production",
						"filename":           "config.json",
						"basenameNormalized": "production",
						"filenameNormalized": "config.json",
						"segments": []string{
							"cluster-config",
							"production",
						},
					},
				},
				{
					"cluster": map[string]any{
						"owner":   "john.doe@example.com",
						"name":    "staging",
						"address": "https://kubernetes.default.svc",
					},
					"path": map[string]any{
						"path":               "cluster-config/production",
						"basename":           "production",
						"filename":           "config.json",
						"basenameNormalized": "production",
						"filenameNormalized": "config.json",
						"segments": []string{
							"cluster-config",
							"production",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:  "Test YAML flow",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.yaml"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.yaml": []byte(`
cluster:
  owner: john.doe@example.com
  name: production
  address: https://kubernetes.default.svc
key1: val1
key2:
  key2_1: val2_1
  key2_2:
    key2_2_1: val2_2_1
`),
				"cluster-config/staging/config.yaml": []byte(`
cluster:
  owner: foo.bar@example.com
  name: staging
  address: https://kubernetes.default.svc
`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster": map[string]any{
						"owner":   "john.doe@example.com",
						"name":    "production",
						"address": "https://kubernetes.default.svc",
					},
					"key1": "val1",
					"key2": map[string]any{
						"key2_1": "val2_1",
						"key2_2": map[string]any{
							"key2_2_1": "val2_2_1",
						},
					},
					"path": map[string]any{
						"path":               "cluster-config/production",
						"basename":           "production",
						"filename":           "config.yaml",
						"basenameNormalized": "production",
						"filenameNormalized": "config.yaml",
						"segments": []string{
							"cluster-config",
							"production",
						},
					},
				},
				{
					"cluster": map[string]any{
						"owner":   "foo.bar@example.com",
						"name":    "staging",
						"address": "https://kubernetes.default.svc",
					},
					"path": map[string]any{
						"path":               "cluster-config/staging",
						"basename":           "staging",
						"filename":           "config.yaml",
						"basenameNormalized": "staging",
						"filenameNormalized": "config.yaml",
						"segments": []string{
							"cluster-config",
							"staging",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:  "test YAML array",
			files: []v1alpha1.GitFileGeneratorItem{{Path: "**/config.yaml"}},
			repoFileContents: map[string][]byte{
				"cluster-config/production/config.yaml": []byte(`
- cluster:
    owner: john.doe@example.com
    name: production
    address: https://kubernetes.default.svc
    inner:
      one: two
- cluster:
    owner: john.doe@example.com
    name: staging
    address: https://kubernetes.default.svc`),
			},
			repoPathsError: nil,
			expected: []map[string]any{
				{
					"cluster": map[string]any{
						"owner":   "john.doe@example.com",
						"name":    "production",
						"address": "https://kubernetes.default.svc",
						"inner": map[string]any{
							"one": "two",
						},
					},
					"path": map[string]any{
						"path":               "cluster-config/production",
						"basename":           "production",
						"filename":           "config.yaml",
						"basenameNormalized": "production",
						"filenameNormalized": "config.yaml",
						"segments": []string{
							"cluster-config",
							"production",
						},
					},
				},
				{
					"cluster": map[string]any{
						"owner":   "john.doe@example.com",
						"name":    "staging",
						"address": "https://kubernetes.default.svc",
					},
					"path": map[string]any{
						"path":               "cluster-config/production",
						"basename":           "production",
						"filename":           "config.yaml",
						"basenameNormalized": "production",
						"filenameNormalized": "config.yaml",
						"segments": []string{
							"cluster-config",
							"production",
						},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, testCase := range cases {
		testCaseCopy := testCase

		t.Run(testCaseCopy.name, func(t *testing.T) {
			t.Parallel()

			argoCDServiceMock := mocks.Repos{}
			argoCDServiceMock.On("GetFiles", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(testCaseCopy.repoFileContents, testCaseCopy.repoPathsError)

			gitGenerator := NewGitGenerator(&argoCDServiceMock, "")
			applicationSetInfo := v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "set",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					GoTemplate: true,
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:  "RepoURL",
							Revision: "Revision",
							Files:    testCaseCopy.files,
						},
					}},
				},
			}

			scheme := runtime.NewScheme()
			err := v1alpha1.AddToScheme(scheme)
			require.NoError(t, err)
			appProject := v1alpha1.AppProject{}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&appProject).Build()

			got, err := gitGenerator.GenerateParams(&applicationSetInfo.Spec.Generators[0], &applicationSetInfo, client)
			fmt.Println(got, err)

			if testCaseCopy.expectedError != nil {
				require.EqualError(t, err, testCaseCopy.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, testCaseCopy.expected, got)
			}

			argoCDServiceMock.AssertExpectations(t)
		})
	}
}

func TestGitGenerator_GenerateParams(t *testing.T) {
	cases := []struct {
		name               string
		appProject         v1alpha1.AppProject
		directories        []v1alpha1.GitDirectoryGeneratorItem
		pathParamPrefix    string
		repoApps           []string
		repoPathsError     error
		repoFileContents   map[string][]byte
		values             map[string]string
		expected           []map[string]any
		expectedError      error
		expectedProject    *string
		appset             v1alpha1.ApplicationSet
		callGetDirectories bool
	}{
		{
			name: "Signature Verification - ignores templated project field",
			repoApps: []string{
				"app1",
			},
			repoPathsError: nil,
			appset: v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "set",
					Namespace: "namespace",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:         "RepoURL",
							Revision:        "Revision",
							Directories:     []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
							PathParamPrefix: "",
							Values: map[string]string{
								"foo": "bar",
							},
						},
					}},
					Template: v1alpha1.ApplicationSetTemplate{
						Spec: v1alpha1.ApplicationSpec{
							Project: "{{.project}}",
						},
					},
				},
			},
			callGetDirectories: true,
			expected:           []map[string]any{{"path": "app1", "path.basename": "app1", "path.basenameNormalized": "app1", "path[0]": "app1", "values.foo": "bar"}},
			expectedError:      nil,
		},
		{
			name: "Signature Verification - Checks for non-templated project field",
			repoApps: []string{
				"app1",
			},
			repoPathsError: nil,
			appset: v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "set",
					Namespace: "namespace",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:         "RepoURL",
							Revision:        "Revision",
							Directories:     []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
							PathParamPrefix: "",
							Values: map[string]string{
								"foo": "bar",
							},
						},
					}},
					Template: v1alpha1.ApplicationSetTemplate{
						Spec: v1alpha1.ApplicationSpec{
							Project: "project",
						},
					},
				},
			},
			callGetDirectories: false,
			expected:           []map[string]any{{"path": "app1", "path.basename": "app1", "path.basenameNormalized": "app1", "path[0]": "app1", "values.foo": "bar"}},
			expectedError:      errors.New("error getting project project: appprojects.argoproj.io \"project\" not found"),
		},
		{
			name: "Project field is not templated - verify that project is passed through to repo-server as-is",
			repoApps: []string{
				"app1",
			},
			callGetDirectories: true,
			appProject: v1alpha1.AppProject{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "project",
					Namespace: "argocd",
				},
			},
			appset: v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "set",
					Namespace: "namespace",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:         "RepoURL",
							Revision:        "Revision",
							Directories:     []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
							PathParamPrefix: "",
							Values: map[string]string{
								"foo": "bar",
							},
						},
					}},
					Template: v1alpha1.ApplicationSetTemplate{
						Spec: v1alpha1.ApplicationSpec{
							Project: "project",
						},
					},
				},
			},
			expected:        []map[string]any{{"path": "app1", "path.basename": "app1", "path.basenameNormalized": "app1", "path[0]": "app1", "values.foo": "bar"}},
			expectedProject: ptr.To("project"),
			expectedError:   nil,
		},
		{
			name: "Project field is templated - verify that project is passed through to repo-server as empty string",
			repoApps: []string{
				"app1",
			},
			callGetDirectories: true,
			appset: v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "set",
					Namespace: "namespace",
				},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{{
						Git: &v1alpha1.GitGenerator{
							RepoURL:         "RepoURL",
							Revision:        "Revision",
							Directories:     []v1alpha1.GitDirectoryGeneratorItem{{Path: "*"}},
							PathParamPrefix: "",
							Values: map[string]string{
								"foo": "bar",
							},
						},
					}},
					Template: v1alpha1.ApplicationSetTemplate{
						Spec: v1alpha1.ApplicationSpec{
							Project: "{{.project}}",
						},
					},
				},
			},
			expected:        []map[string]any{{"path": "app1", "path.basename": "app1", "path.basenameNormalized": "app1", "path[0]": "app1", "values.foo": "bar"}},
			expectedProject: ptr.To(""),
			expectedError:   nil,
		},
	}
	for _, testCase := range cases {
		argoCDServiceMock := mocks.Repos{}

		if testCase.callGetDirectories {
			var project any
			if testCase.expectedProject != nil {
				project = *testCase.expectedProject
			} else {
				project = mock.Anything
			}

			argoCDServiceMock.On("GetDirectories", mock.Anything, mock.Anything, mock.Anything, project, mock.Anything, mock.Anything).Return(testCase.repoApps, testCase.repoPathsError)
		}
		gitGenerator := NewGitGenerator(&argoCDServiceMock, "argocd")

		scheme := runtime.NewScheme()
		err := v1alpha1.AddToScheme(scheme)
		require.NoError(t, err)

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&testCase.appProject).Build()

		got, err := gitGenerator.GenerateParams(&testCase.appset.Spec.Generators[0], &testCase.appset, client)

		if testCase.expectedError != nil {
			require.EqualError(t, err, testCase.expectedError.Error())
		} else {
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, got)
		}

		argoCDServiceMock.AssertExpectations(t)
	}
}
