package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

const APP_NAME = "trakt-cli"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   APP_NAME,
	Short: "A nice CLI tool for trakt.tv",
	Long:  `Source code: https://github.com/shizeeg/trakt-cli`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	confDir, err := os.UserConfigDir()
	if err != nil {
		confDir, _ = os.UserHomeDir()
	}
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", fmt.Sprintf("config file (default is %q)", filepath.Join(confDir, APP_NAME, "config.yaml")))

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find $XDG_CONFIG_HOME directory.
		xdgCfgDir, err := os.UserConfigDir()
		cobra.CheckErr(err)

		// Find project subdirectory in $XDG_CONFIG_HOME and set filename to config.yaml
		viper.AddConfigPath(filepath.Join(xdgCfgDir, APP_NAME))
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		log.Println(err)
	}
}
