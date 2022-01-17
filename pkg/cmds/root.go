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
	"io"

	"github.com/spf13/cobra"
	v "gomodules.xyz/x/version"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"kmodules.xyz/client-go/tools/cli"
)

func NewKubeformCmd(in io.Reader, out, err io.Writer) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               "kf [command]",
		Short:             `Kubeform cli by AppsCode`,
		DisableAutoGenTag: true,
		PersistentPreRun: func(c *cobra.Command, args []string) {
			cli.SendAnalytics(c, v.Version.Version)
		},
	}

	flags := rootCmd.PersistentFlags()

	kubeConfigFlags := genericclioptions.NewConfigFlags(true)
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(flags)

	flags.BoolVar(&cli.EnableAnalytics, "enable-analytics", cli.EnableAnalytics, "Send analytical events to Google Analytics")

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	ioStreams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: err}

	rootCmd.AddCommand(NewCmdCompletion())
	rootCmd.AddCommand(v.NewCmdVersion())
	rootCmd.AddCommand(NewCmdGetTF("kf", f, ioStreams))
	rootCmd.AddCommand(NewCmdGenModule("kf", f))

	return rootCmd
}
