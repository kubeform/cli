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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type GetTFOptions struct {
	CmdParent           string
	Namespace           string
	Directory           string
	OperatorNamespace   string
	OperatorServiceName string

	Config *rest.Config

	NewBuilder func() *resource.Builder

	BuilderArgs []string

	genericclioptions.IOStreams
}

func NewCmdGetTF(parent string, f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	var directory, operatorNamespace, operatorServiceName string

	cmd := &cobra.Command{
		Use:               "get-tf",
		Short:             "Get the tf and tfstate of kubeform resource",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := &GetTFOptions{
				CmdParent:           parent,
				IOStreams:           streams,
				Directory:           directory,
				OperatorNamespace:   operatorNamespace,
				OperatorServiceName: operatorServiceName,
			}
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Run())
			return nil
		},
	}

	cmd.Flags().StringVarP(&directory, "directory", "d", "", "directory where tf and tfstate should store")
	cmd.Flags().StringVar(&operatorNamespace, "operator-ns", "kubeform", "namespace where respective cloud provider's kubeform operator is installed")
	cmd.Flags().StringVar(&operatorServiceName, "operator-svc", "", "respective cloud provider's kubeform operator service name")

	return cmd
}

func (o *GetTFOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	var err error
	o.Namespace, _, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	if len(args) != 2 {
		return fmt.Errorf("You must specify the type of resource and name of the resource to get tf. %s\n", cmdutil.SuggestAPIResources(o.CmdParent))
	}

	o.BuilderArgs = args

	o.NewBuilder = f.NewBuilder

	o.Config, err = f.ToRESTConfig()
	if err != nil {
		return err
	}

	return nil
}

func (o *GetTFOptions) Validate(args []string) error {
	return nil
}

func (o *GetTFOptions) Run() error {
	r := o.NewBuilder().
		Unstructured().
		ContinueOnError().
		NamespaceParam(o.Namespace).DefaultNamespace().
		ResourceTypeOrNameArgs(true, o.BuilderArgs...).
		Flatten().
		Do()
	err := r.Err()
	if err != nil {
		return err
	}

	infos, err := r.Infos()
	if err != nil {
		return err
	}

	if len(infos) == 0 {
		return fmt.Errorf("no resources found")
	}

	gvr := infos[0].Mapping.Resource
	resourceName := o.BuilderArgs[1]
	reSource := gvr.Resource
	group := gvr.Group
	version := gvr.Version
	temp := strings.Split(group, ".")
	providerName := temp[1]

	tr, err := rest.TransportFor(o.Config)
	if err != nil {
		return err
	}

	client := &http.Client{Transport: tr}
	jsn := []byte(`{
					"namespace": "` + o.Namespace + `", 
					"resource-name": "` + resourceName + `", 
					"group": "` + group + `", 
					"version": "` + version + `", 
					"resource": "` + reSource + `"
			}`)
	buf := bytes.NewBuffer(jsn)

	operatorServiceName := "kubeform-provider-" + providerName + "-webhook-server"
	if o.OperatorServiceName != "" {
		operatorServiceName = o.OperatorServiceName
	}
	operatorNamespace := o.OperatorNamespace

	url := o.Config.Host + "/api/v1/namespaces/" + operatorNamespace + "/services/https:" + operatorServiceName + ":/proxy/tf"

	req, err := http.NewRequest(http.MethodPost, url, buf)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	bdy, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	tmp := make(map[string]string)
	err = json.Unmarshal(bdy, &tmp)
	if err != nil {
		return fmt.Errorf(err.Error(), string(bdy))
	}

	tf := tmp["tf"]
	tfstate := tmp["tfstate"]
	var tempInterface interface{}
	err = json.Unmarshal([]byte(tfstate), &tempInterface)
	if err != nil {
		return err
	}
	tfstateByte, err := json.MarshalIndent(tempInterface, "", "\t")
	if err != nil {
		return err
	}

	directory := o.Directory
	if directory == "" {
		fmt.Println("tf is : ")
		fmt.Println(tf)
		fmt.Println("tfstate is : ")
		fmt.Println(string(tfstateByte))
	} else {
		err := os.WriteFile(filepath.Join(directory, "main.tf"), []byte(tf), 0o777)
		if err != nil {
			return err
		}
		fmt.Println("main.tf is Successfully generated!")
		err = os.WriteFile(filepath.Join(directory, "terraform.tfstate"), tfstateByte, 0o777)
		if err != nil {
			return err
		}
		fmt.Println("terraform.tfstate is Successfully generated!")
	}

	return nil
}
