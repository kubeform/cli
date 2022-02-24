/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmds

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kubeform.dev/module/api/v1alpha1"

	"github.com/ghodss/yaml"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/resource"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	apiv1 "kmodules.xyz/client-go/api/v1"
)

const (
	Bool   = "bool"
	String = "string"
	Number = "number"
)

type GenModuleOptions struct {
	CmdParent          string
	ModuleDefName      string
	ProviderName       string
	ProviderSource     string
	Directory          string
	Token              string
	Source             string
	Ref                string
	Apply              bool
	GenSecretNamespace string

	NewBuilder func() *resource.Builder

	BuilderArgs []string
}

func NewCmdGenModule(parent string, f cmdutil.Factory) *cobra.Command {
	var directory, providerName, providerSource, source, token, genSecretNamespace, ref string
	var apply bool

	cmd := &cobra.Command{
		Use:               "gen-module",
		Short:             "Generate the module definition of given module",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := &GenModuleOptions{
				CmdParent:          parent,
				ModuleDefName:      args[0],
				ProviderName:       providerName,
				ProviderSource:     providerSource,
				Directory:          directory,
				Ref:                ref,
				Token:              token,
				GenSecretNamespace: genSecretNamespace,
				Source:             source,
				Apply:              apply,
			}
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Run())
			return nil
		},
	}

	cmd.Flags().StringVar(&directory, "directory", "", "directory where generated module definition and git cred secret should store")
	cmd.Flags().StringVar(&token, "token", "", "personal access token for cloning private github module repo")
	cmd.Flags().StringVar(&genSecretNamespace, "secret-namespace", "default", "namespace where git cred secret will be generated by Kubeform CLI")
	cmd.Flags().StringVar(&source, "source", "", "source where module tf files are located")
	cmd.Flags().StringVar(&providerName, "provider-name", "", "module's provider name")
	cmd.Flags().StringVar(&providerSource, "provider-source", "", "module's provider source")
	cmd.Flags().BoolVarP(&apply, "apply", "a", false, "whether we want to apply the generated Module Definition or not")
	cmd.Flags().StringVar(&ref, "ref", "", "ref for doing git checkout")

	return cmd
}

func (o *GenModuleOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("you must specify the name of the module to generate Module Definition")
	} else if len(args) != 1 {
		return fmt.Errorf("you must specify only the name of the module to generate Module Definition")
	}

	o.BuilderArgs = args

	o.NewBuilder = f.NewBuilder

	return nil
}

func (o *GenModuleOptions) Validate(args []string) error {
	return nil
}

func (o *GenModuleOptions) Run() error {
	err := generateModuleTRD(o.Source, o.ModuleDefName, o.ProviderName, o.ProviderSource, o.Directory, o.Token, o.Apply, o.GenSecretNamespace, o.Ref)
	if err != nil {
		return err
	}

	return nil
}

func generateModuleTRD(source, moduleDefName, providerName, providerSource, directory, token string, apply bool, credSecretNamespace, ref string) error {
	modifiedUrl, err := url.Parse(source)
	if err != nil {
		return err
	}
	source = modifiedUrl.Host + modifiedUrl.Path
	sourceSlice := strings.Split(source, "/")
	if len(sourceSlice) == 0 {
		return fmt.Errorf("given github repo source link is invalid")
	}
	repoName := sourceSlice[len(sourceSlice)-1]
	hostName := sourceSlice[0]

	path := filepath.Join("/tmp", moduleDefName)
	err = createGitRepoTempPath(path)
	if err != nil {
		return err
	}

	src := source
	var credSecretName string
	secretObj := corev1.Secret{}

	if token != "" {
		// for bitbucket token need to be in the format of "username:app-password"
		// for github and gitlab it's only the personal access token
		if strings.Contains(hostName, "github.com") || strings.Contains(hostName, "bitbucket.org") {
			src = "https://" + token + "@" + src + ".git"
		} else if strings.Contains(hostName, "gitlab.com") {
			src = "https://oauth2:" + token + "@" + src + ".git"
		}

		credSecretName = moduleDefName + "-git-cred"

		secretObj = corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      credSecretName,
				Namespace: credSecretNamespace,
			},
			Data: map[string][]byte{
				"token": []byte(token),
			},
		}
	} else {
		src = "https://" + src + ".git"
	}

	repoPath := filepath.Join(path, repoName)
	err = gitRepoClone(path, src, repoPath)
	if err != nil {
		return err
	}

	err = checkGitRef(repoPath, ref)
	if err != nil {
		return err
	}

	if tfconfig.IsModuleDir(repoPath) {
		module, diag := tfconfig.LoadModule(repoPath)
		if diag.HasErrors() {
			return diag.Err()
		}

		variables := module.Variables
		outputs := module.Outputs

		var varKeys []string
		for k := range variables {
			varKeys = append(varKeys, k)
		}

		var outKeys []string
		for k := range outputs {
			outKeys = append(outKeys, k)
		}

		input, required, err := processInput(varKeys, variables)
		if err != nil {
			return err
		}
		output, err := processOutput(outKeys, outputs)
		if err != nil {
			return err
		}

		jsonSchemaProps := v1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]v1.JSONSchemaProps{
				"input": {
					Type:       "object",
					Properties: input,
					Required:   required,
				},
				"output": {
					Type:       "object",
					Properties: output,
				},
			},
			Required: []string{
				"input",
			},
		}

		modObj := v1alpha1.ModuleDefinition{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ModuleDefinition",
				APIVersion: v1alpha1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: moduleDefName,
			},
			Spec: v1alpha1.ModuleDefinitionSpec{
				Schema: jsonSchemaProps,
				ModuleRef: v1alpha1.ModuleRef{
					Git: v1alpha1.Git{
						Ref: source,
					},
				},
				Provider: v1alpha1.Provider{
					Name:   providerName,
					Source: providerSource,
				},
			},
		}

		var secretYaml []byte
		if credSecretName != "" {
			modObj.Spec.ModuleRef.Git.Cred = &apiv1.ObjectReference{
				Namespace: credSecretNamespace,
				Name:      credSecretName,
			}

			secretYaml, err = yaml.Marshal(secretObj)
			if err != nil {
				return err
			}
		}
		if ref != "" {
			modObj.Spec.ModuleRef.Git.CheckOut = &ref
		}

		modYml, err := yaml.Marshal(modObj)
		if err != nil {
			return err
		}

		modDefYamlPath := filepath.Join(directory, moduleDefName+".yaml")
		err = os.WriteFile(modDefYamlPath, modYml, 0774)
		if err != nil {
			return err
		}

		var secretYamlPath string
		if credSecretName != "" {
			secretYamlPath = filepath.Join(directory, credSecretName+".yaml")
			err = os.WriteFile(secretYamlPath, secretYaml, 0774)
			if err != nil {
				return err
			}
		}

		if apply {
			if err := yamlsApply(modDefYamlPath); err != nil {
				return err
			}

			if credSecretName != "" {
				if err = yamlsApply(secretYamlPath); err != nil {
					return err
				}
			}
		}

		return nil
	}

	return fmt.Errorf("no terraform configuration file is found in the path : %v\n", path)
}

func createGitRepoTempPath(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, 0777)
		if err != nil {
			return err
		}
	}

	return nil
}

func gitRepoClone(path, src, repoPath string) error {
	_, err := os.Stat(repoPath)
	if os.IsNotExist(err) {
		cmd := exec.Command("git", "clone", src)
		cmd.Dir = path
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

func checkGitRef(path, ref string) error {
	if ref == "" {
		return nil
	}

	cmd := exec.Command("git", "checkout", ref)
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func processInput(keys []string, variables map[string]*tfconfig.Variable) (map[string]v1.JSONSchemaProps, []string, error) {
	mp := map[string]v1.JSONSchemaProps{}
	var required []string

	for _, key := range keys {
		variable := variables[key]

		if variable.Required {
			required = append(required, key)
		}
	}

	for _, key := range keys {
		variable := variables[key]

		if variable.Type == "" || variable.Type == "any" {
			mp[key] = v1.JSONSchemaProps{
				Description: variable.Description,
				AnyOf: []v1.JSONSchemaProps{
					{
						Type: Number,
					},
					{
						Type: String,
					},
					{
						Type: "object",
					},
				},
			}
		} else if variable.Type == Number {
			mp[key] = v1.JSONSchemaProps{
				Type:        Number,
				Description: variable.Description,
			}
		} else if variable.Type == String {
			mp[key] = v1.JSONSchemaProps{
				Type:        String,
				Description: variable.Description,
			}
		} else if variable.Type == Bool {
			mp[key] = v1.JSONSchemaProps{
				Type:        "boolean",
				Description: variable.Description,
			}
		} else if strings.Contains(variable.Type, "list") || strings.Contains(variable.Type, "set") {
			typ := strings.FieldsFunc(variable.Type, func(r rune) bool {
				return r == '(' || r == ')'
			})

			if len(typ) == 1 {
				mp[key] = v1.JSONSchemaProps{
					Type:        "array",
					Description: variable.Description,
					Items: &v1.JSONSchemaPropsOrArray{
						Schema: &v1.JSONSchemaProps{
							AnyOf: []v1.JSONSchemaProps{
								{
									Type: Number,
								},
								{
									Type: String,
								},
								{
									Type: "object",
								},
							},
						},
					},
				}
			} else if typ[1] == Bool {
				mp[key] = v1.JSONSchemaProps{
					Type:        "array",
					Description: variable.Description,
					Items: &v1.JSONSchemaPropsOrArray{
						Schema: &v1.JSONSchemaProps{
							Type: "boolean",
						},
					},
				}
			} else if typ[1] == Number {
				mp[key] = v1.JSONSchemaProps{
					Type:        "array",
					Description: variable.Description,
					Items: &v1.JSONSchemaPropsOrArray{
						Schema: &v1.JSONSchemaProps{
							Type: Number,
						},
					},
				}
			} else if typ[1] == String {
				mp[key] = v1.JSONSchemaProps{
					Type:        "array",
					Description: variable.Description,
					Items: &v1.JSONSchemaPropsOrArray{
						Schema: &v1.JSONSchemaProps{
							Type: String,
						},
					},
				}
			} else {
				return nil, nil, fmt.Errorf("not supported vairable, name: %s and type: %s\n", variable.Name, variable.Type)
			}
		} else if strings.Contains(variable.Type, "map") {
			typ := strings.FieldsFunc(variable.Type, func(r rune) bool {
				return r == '(' || r == ')'
			})

			if typ[1] == Bool {
				mp[key] = v1.JSONSchemaProps{
					Type:        "object",
					Description: variable.Description,
					AdditionalProperties: &v1.JSONSchemaPropsOrBool{
						Schema: &v1.JSONSchemaProps{
							Type: "boolean",
						},
					},
				}
			} else if typ[1] == Number {
				mp[key] = v1.JSONSchemaProps{
					Type:        "object",
					Description: variable.Description,
					AdditionalProperties: &v1.JSONSchemaPropsOrBool{
						Schema: &v1.JSONSchemaProps{
							Type: Number,
						},
					},
				}
			} else if typ[1] == String {
				mp[key] = v1.JSONSchemaProps{
					Type:        "object",
					Description: variable.Description,
					AdditionalProperties: &v1.JSONSchemaPropsOrBool{
						Schema: &v1.JSONSchemaProps{
							Type: String,
						},
					},
				}
			} else {
				return nil, nil, fmt.Errorf("not supported vairable, name: %s and type: %s\n", variable.Name, variable.Type)
			}
		} else {
			return nil, nil, fmt.Errorf("not supported vairable, name: %s and type: %s\n", variable.Name, variable.Type)
		}
	}

	return mp, required, nil
}

func processOutput(keys []string, outputs map[string]*tfconfig.Output) (map[string]v1.JSONSchemaProps, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("no output is defined in the module path")
	}

	mp := make(map[string]v1.JSONSchemaProps)

	for _, key := range keys {
		output := outputs[key]

		mp[key] = v1.JSONSchemaProps{
			Description: output.Description,
			AnyOf: []v1.JSONSchemaProps{
				{
					Type: Number,
				},
				{
					Type: String,
				},
				{
					Type: "boolean",
				},
				{
					Type: "object",
				},
			},
		}
	}

	return mp, nil
}

func yamlsApply(filePath string) error {
	cmd := exec.Command("kubectl", "apply", "-f", filePath)
	cmd.Dir = filePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
