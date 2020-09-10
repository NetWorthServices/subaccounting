package subaccounting

import (
	"fmt"

	"git.aax.dev/agora-altx/models-go/networth"
)

// Execute transaction by Transfer as args
func (payload *Subledger) Execute(args Transfer) (results []networth.Investor) {

	switch args.Type {
	case "FIFO":
		results = fifo(payload, args)
		break
	default:
		results = fifo(payload, args)
	}

	return
}

// FIFO returns withdrawl amounts sequentially by Transfer
func fifo(subledger *Subledger, args Transfer) (sequence []networth.Investor) {
	// Get investment count
	tc := len(subledger.Investments)

	// Remaining amount to subtract from accounts
	deficit := args.Amount

	// Amount subtracted from previous account
	// prevDeficit := args.Amount

	// Amount subtracted from current account
	subtraction := 0.00

	// Loop through transaction sequentially
	for i := 0; i < tc; i++ {

		/*
			If transaction is a transfer into the account
			specified in r.To subtract the current amount
			from the r and continue unti r == 0
		*/
		if subledger.Investments[i].Amount > 0.00 {
			fmt.Printf("Amt: $%f   Def: $%f\n", subledger.Investments[i].Amount, deficit)
			if subledger.Investments[i].Amount-deficit >= 0.00 {
				// Amount is greater than deficit so subtract only the difference
				subtraction = deficit
				deficit = 0

			} else {
				// Amount is less than deficit so subtract entire transaction
				subtraction = subledger.Investments[i].Amount
				deficit -= subtraction
			}

			// Push values to sequence
			sequence = append(sequence, networth.Investor{
				PathchainID:     subledger.Investments[i].PathchainID,
				InvestorAccount: subledger.Investments[i].InvestorAccount,
				Amount:          subtraction,
				Timestamp:       args.Timestamp,
			})

			subledger.Investments[i].Amount -= subtraction

			// If deficit is satisfied break
			if deficit <= 0.00 {
				break
			}
		}
	}

	return
}
