/*
Copyright 2018 The Kubernetes Authors.

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

// Tests individual AWS actuator actions. This is meant to be executed
// in a machine that has access to AWS either as an instance with the right role
// or creds in ~/.aws/credentials

import (
	"context"
	goflag "flag"
	"fmt"
	"os"
	"os/user"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	"k8s.io/client-go/kubernetes/scheme"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func init() {
	// Add types to scheme
	clusterv1.AddToScheme(scheme.Scheme)

	rootCmd.PersistentFlags().StringP("machine", "m", "", "Machine manifest")
	rootCmd.PersistentFlags().StringP("cluster", "c", "", "Cluster manifest")

	rootCmd.PersistentFlags().StringP("kubeconfig", "", "", "Kubeconfig")
	cUser, err := user.Current()
	if err != nil {
		rootCmd.PersistentFlags().StringP("environment-id", "p", "", "Directory with bootstrapping manifests")
	} else {
		rootCmd.PersistentFlags().StringP("environment-id", "p", cUser.Username, "Machine prefix, by default set to the current user")
	}

	rootCmd.AddCommand(createCommand())

	rootCmd.AddCommand(deleteCommand())

	rootCmd.AddCommand(existsCommand())

	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	// the following line exists to make glog happy, for more information, see: https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})
}

func usage() {
	fmt.Printf("Usage: %s\n\n", os.Args[0])
}

func checkFlags(cmd *cobra.Command) error {
	if cmd.Flag("cluster").Value.String() == "" {
		return fmt.Errorf("--%v/-%v flag is required", cmd.Flag("cluster").Name, cmd.Flag("cluster").Shorthand)
	}
	if cmd.Flag("machine").Value.String() == "" {
		return fmt.Errorf("--%v/-%v flag is required", cmd.Flag("machine").Name, cmd.Flag("machine").Shorthand)
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "aws-actuator-test",
	Short: "Test for Cluster API AWS actuator",
}

func createCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create machine instance for specified cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, err := readClusterResources(
				&manifestParams{
					ClusterID: cmd.Flag("environment-id").Value.String(),
				},
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
			)
			if err != nil {
				return fmt.Errorf("unable to create read resources: %v", err)
			}

			actuator, err := createActuator(machine, cmd.Flag("kubeconfig").Value.String())
			if err != nil {
				return fmt.Errorf("unable to create actuator: %v", err)
			}
			result, err := actuator.CreateMachine(cluster, machine)
			if err != nil {
				return fmt.Errorf("unable to create machine: %v", err)
			}
			fmt.Printf("Machine creation was successful! Pod %s/%s\n", result.Namespace, result.Name)
			return nil
		},
	}
}

func deleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Delete machine instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, err := readClusterResources(
				&manifestParams{
					ClusterID: cmd.Flag("environment-id").Value.String(),
				},
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
			)
			if err != nil {
				return err
			}

			if err != nil {
				return fmt.Errorf("unable to create read resources: %v", err)
			}

			actuator, err := createActuator(machine, cmd.Flag("kubeconfig").Value.String())
			if err != nil {
				return fmt.Errorf("unable to create actuator: %v", err)
			}
			if err = actuator.DeleteMachine(cluster, machine); err != nil {
				return fmt.Errorf("unable to delete machine: %v", err)
			}

			fmt.Printf("Machine delete operation was successful.\n")
			return nil
		},
	}
}

func existsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "exists",
		Short: "Determine if underlying machine instance exists",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, err := readClusterResources(
				&manifestParams{
					ClusterID: cmd.Flag("environment-id").Value.String(),
				},
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
			)
			if err != nil {
				return fmt.Errorf("unable to create read resources: %v", err)
			}

			actuator, err := createActuator(machine, cmd.Flag("kubeconfig").Value.String())
			if err != nil {
				return fmt.Errorf("unable to create actuator: %v", err)
			}
			exists, err := actuator.Exists(context.TODO(), cluster, machine)
			if err != nil {
				return fmt.Errorf("unable to check if machine exists: %v", err)
			}
			if exists {
				fmt.Printf("Underlying machine's instance exists.\n")
			} else {
				fmt.Printf("Underlying machine's instance not found.\n")
			}
			return nil
		},
	}
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}
