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

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func NewCmdGettf() *cobra.Command {
	var namespace, directory string

	cmd := &cobra.Command{
		Use:               "get-tf",
		Short:             "Get the tf and tfstate of kubeform resource",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var kubeconfig *string
			temp := filepath.Join(homedir.HomeDir(), ".kube/config")
			kubeconfig = &temp

			config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
			if err != nil {
				return err
			}

			hst := config.Host
			resourceName := args[1]
			resource := args[0]
			group := "instance.linode.kubeform.com"
			version := "v1alpha1"

			tr, err := rest.TransportFor(config)
			if err != nil {
				return err
			}

			client := &http.Client{Transport: tr}
			jsn := []byte(`{
					"namespace": "` + namespace + `", 
					"resource-name": "` + resourceName + `", 
					"group": "` + group + `", 
					"version": "` + version + `", 
					"resource": "` + resource + `"
			}`)
			buf := bytes.NewBuffer(jsn)

			url := hst + "/api/v1/namespaces/kubeform/services/https:kubeform-provider-linode-webhook-server:/proxy/tf"

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
			fmt.Println(string(bdy))

			tmp := make(map[string]string)
			err = json.Unmarshal(bdy, &tmp)
			if err != nil {
				return err
			}

			tf := tmp["tf"]
			tfstate := tmp["tfstate"]

			if directory == "" {
				fmt.Println("tf : ")
				fmt.Println(tf)
				fmt.Println("tfstate : ")
				fmt.Println(tfstate)
			} else {
				err := os.WriteFile(directory+"/main.tf", []byte(tf), 0777)
				if err != nil {
					return err
				}
				err = os.WriteFile(directory+"/terraform.tfstate", []byte(tfstate), 0777)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "ns", "demo", "resource's namespace name")
	cmd.Flags().StringVarP(&directory, "directory", "dir", "", "directory where tf and tfstate should store")
	//cmd.Flags().StringVar(&group, "group", "", "API Group of the resource")
	//cmd.Flags().StringVar(&version, "version", "v1alpha1", "Version of the resource")
	//cmd.Flags().StringVar(&resource, "resource", "", "")

	return cmd
}
