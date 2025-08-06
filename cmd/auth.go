package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shizeeg/trakt-cli/api"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// authCmd represents the auth command
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with trakt.tv",
	Long:  "You will need to go to https://trakt.tv/oauth/applications/new to get a access id and secret",
	Run: func(cmd *cobra.Command, args []string) {

		p := tea.NewProgram(initialModel())
		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
	},
}

func traktAuthenticate(ClientID string, ClientSecret string) (err error) {
	client := api.NewAPIClient()
	resp, err := client.AuthDeviceCode(&api.AuthDeviceCodeReq{ClientID: ClientID})

	if err != nil {
		log.Fatalf("Failed to get device code: %v\n", err)
		return
	}

	fmt.Printf("Please go to %q and enter the following code: %s\n", resp.VerificationURL, resp.UserCode)
	s := spinner.New(spinner.CharSets[2], 100*time.Millisecond)
	s.Start()
	s.Prefix = "Waiting for authorisation..."
	for {
		tokenResp, err := client.AuthDeviceToken(
			&api.AuthDeviceTokenReq{
				Code:         resp.DeviceCode,
				ClientID:     ClientID,
				ClientSecret: ClientSecret,
			})
		if err != nil {
			log.Fatalf("Failed to get device code: %s\n", err)
			return err
		}

		if tokenResp == nil || len(tokenResp.AccessToken) == 0 {
			time.Sleep(time.Duration(resp.Interval) * time.Second)
		} else {
			viper.Set("trakt.client-id", ClientID)
			viper.Set("trakt.client-secret", ClientSecret)
			viper.Set("trakt.access-token", tokenResp.AccessToken)
			viper.Set("trakt.refresh-token", tokenResp.RefreshToken)
			viper.WriteConfig()

			s.Stop()
			log.Printf("Successfully authenticated, creds written to %q\n", viper.ConfigFileUsed())

			break
		}
	}
	return
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(authCmd)
	authCmd.PersistentFlags().String("client-id", "", "")
	authCmd.PersistentFlags().String("client-secret", "", "")
	viper.BindPFlag("client-id", authCmd.PersistentFlags().Lookup("client-id"))
	viper.BindPFlag("client-secret", authCmd.PersistentFlags().Lookup("client-secret"))
}

func initConfig() {
	xdgConfDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal("Can't read config directory: ", err)
	}
	xdgConfDir = filepath.Join(xdgConfDir, "trakt-cli")

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(xdgConfDir)

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Can't read config: ", err)
	}
}

// UI code
type model struct {
	inputs       []textinput.Model
	focused      int
	submitted    bool
	clientID     string
	clientSecret string
}

func initialModel() model {
	inputs := make([]textinput.Model, 2)
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "ClientID"
	inputs[0].Focus()
	inputs[0].CharLimit = 70
	inputs[0].Width = 78
	inputs[0].Prompt = "> "

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "ClientSecret"
	inputs[1].CharLimit = 70
	inputs[1].Width = 78
	inputs[1].Prompt = "> "
	// inputs[1].EchoMode = textinput.EchoPassword
	// inputs[1].EchoCharacter = '•'

	return model{
		inputs:       inputs,
		focused:      0,
		submitted:    false,
		clientID:     "",
		clientSecret: "",
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.focused == len(m.inputs)-1 {
				m.submitted = true
				m.clientID = m.inputs[0].Value()
				m.clientSecret = m.inputs[1].Value()
				if err := traktAuthenticate(m.clientID, m.clientSecret); err != nil {
					log.Fatal(err)
				}
				return m, tea.Quit
			}
			m.nextInput()
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyShiftTab, tea.KeyCtrlP:
			m.prevInput()
		case tea.KeyTab, tea.KeyCtrlN:
			m.nextInput()
		}
	}

	// Handle character input and blinking
	cmd := m.updateInputs(msg)

	return m, cmd
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	// Only update the focused input
	for i := range m.inputs {
		if i == m.focused {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return tea.Batch(cmds...)
}

func (m *model) nextInput() {
	m.focused = (m.focused + 1) % len(m.inputs)
	for i := 0; i < len(m.inputs); i++ {
		if i == m.focused {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m *model) prevInput() {
	m.focused--
	if m.focused < 0 {
		m.focused = len(m.inputs) - 1
	}
	for i := 0; i < len(m.inputs); i++ {
		if i == m.focused {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m model) View() string {
	if m.submitted {
		return fmt.Sprintf(
			"ClientID: %q\nClientSecret: %q\n",
			m.clientID,
			m.clientSecret,
		)
	}

	var b strings.Builder

	b.WriteString("Enter TraktTV credentials\n")

	for i := range m.inputs {
		b.WriteString(m.inputs[i].View())
		b.WriteRune('\n')
	}

	b.WriteString("(tab/shift+tab to switch fields, ctrl+c to quit)\n")
	b.WriteString("(press <Enter> to submit)\n")

	return b.String()
}
