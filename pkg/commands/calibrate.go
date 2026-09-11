package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	calibrateStartOnly bool
	calibrateStopOnly  bool
)

const calibrateInstructions = `Calibration started. Now, with the controller on a flat surface:

  1. Leave it still for about 3 seconds (gyro).
  2. Push each joystick all the way out and rotate it slowly twice.
  3. Let the joysticks return to the center.
  4. Pull both triggers all the way in and release, twice.

Do not press any other buttons.`

var calibrateCommand = &cobra.Command{
	Use:   "calibrate",
	Short: "Calibrate joysticks, triggers and gyro",
	Long: `Runs the controller's built-in calibration procedure (the same one Flydigi
Space Station starts from its "joystick calibration" dialog).

The controller enters calibration mode, records the full travel of the joysticks
and triggers while you move them, and stores the result when calibration is
finished. Use --start / --stop to drive the two steps separately from a script.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if calibrateStartOnly && calibrateStopOnly {
			return errors.New("--start and --stop are mutually exclusive")
		}

		return useConnection(func() error {
			if calibrateStopOnly {
				if err := dbusClient.Calibrate(false); err != nil {
					return fmt.Errorf("finish calibration: %w", err)
				}
				if !terseOutput {
					fmt.Println("Calibration finished.")
				}
				return nil
			}

			if err := dbusClient.Calibrate(true); err != nil {
				return fmt.Errorf("start calibration: %w", err)
			}

			if calibrateStartOnly {
				if !terseOutput {
					fmt.Println("Calibration started. Run `flydigictl calibrate --stop` when done.")
				}
				return nil
			}

			fmt.Println(calibrateInstructions)
			fmt.Println()
			fmt.Print("Press Enter when done (Ctrl+C aborts and also finishes calibration)... ")

			done := make(chan struct{})
			go func() {
				bufio.NewReader(os.Stdin).ReadString('\n')
				close(done)
			}()

			select {
			case <-done:
			case <-cmd.Context().Done():
			}

			// Give the controller a moment to settle before storing
			time.Sleep(500 * time.Millisecond)

			if err := dbusClient.Calibrate(false); err != nil {
				return fmt.Errorf("finish calibration: %w", err)
			}

			if !terseOutput {
				fmt.Println("Calibration finished.")
			}

			return nil
		})
	},
}

func init() {
	rootCmd.AddCommand(calibrateCommand)

	calibrateCommand.Flags().BoolVar(&calibrateStartOnly, "start", false, "only start calibration mode and exit")
	calibrateCommand.Flags().BoolVar(&calibrateStopOnly, "stop", false, "only finish calibration mode and exit")
}
