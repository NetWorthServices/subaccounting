package subaccounting

import (
	"time"

	"git.aax.dev/agora-altx/models-go/networth"
	"git.aax.dev/agora-altx/utils-go/util"
)

func (transaction *Transaction) processFundEvents(meta networth.ActivityMetaData) {
	trn := *transaction

	if trn.ExecuteType == networth.ETSubscription || trn.ExecuteType == networth.ETExternalSubscription {
		if meta.IsQualifiedCapitalGains() {
			bankDate := trn.Timestamp
			date2026, _ := time.Parse(util.DateFormat("Y-m-d"), "2026-12-31")

			trn.CapitalAccount = trn.Amount
			trn.CostBasis = 0.00
			trn.addEvent(networth.EventCalculationEntry{
				Entry:          "Initial Investment is a Qualified Investment",
				Editable:       false,
				CapitalAccount: trn.Amount,
				CostBasis:      0.00,
			})
			if time.Now().After(bankDate.AddDate(5, 0, 0)) && bankDate.AddDate(5, 0, 0).Before(date2026) {
				cb10 := util.RoundMoney(trn.Amount * 0.1)
				trn.CostBasis += cb10
				trn.addEvent(networth.EventCalculationEntry{
					Entry:          "Five Year step up",
					Editable:       false,
					CapitalAccount: 0.00,
					CostBasis:      cb10,
				})
			}
			if time.Now().After(bankDate.AddDate(7, 0, 0)) && bankDate.AddDate(7, 0, 0).Before(date2026) {
				cb5 := util.RoundMoney(trn.Amount * 0.05)
				trn.CostBasis += cb5
				trn.addEvent(networth.EventCalculationEntry{
					Entry:          "Seven Year step up",
					Editable:       false,
					CapitalAccount: 0.00,
					CostBasis:      cb5,
				})
			}
			if time.Now().After(date2026) {
				tp2026 := trn.Amount - trn.CostBasis
				trn.CostBasis = trn.Amount
				trn.addEvent(networth.EventCalculationEntry{
					Entry:          "2026 Taxes Paid",
					Editable:       false,
					CapitalAccount: 0.00,
					CostBasis:      tp2026,
				})
			}
		} else {
			trn.CapitalAccount = trn.Amount
			trn.CostBasis = trn.Amount
			trn.addEvent(networth.EventCalculationEntry{
				Entry:          "Initial Investment is a Non-Qualified Investment",
				Editable:       false,
				CapitalAccount: trn.CapitalAccount,
				CostBasis:      trn.CostBasis,
			})
		}
	}

	if trn.ExecuteType == networth.ETDebt || trn.ExecuteType == networth.ETExternalDebt {
		// TODO: This is just a stub to handle the Debt side of things for the Event Manager
		a := networth.FindAccount(trn.To)
		e := networth.FindEntity(a.IDEntity)
		trn.CapitalAccount = 0.00
		trn.CostBasis = 0.00

		if len(trn.Guarantors) > 0 {
			// If the debt has gauarantors, add the cost basis to the gaurantors
			for _, g := range trn.Guarantors {
				trn.addEvent(networth.EventCalculationEntry{
					IDEntity:       g.EntityID,
					Entry:          "Debt",
					Editable:       false,
					CapitalAccount: trn.CapitalAccount,
					CostBasis:      util.Float64FromString(g.Amount),
				})
			}

		} else if e.DetailJSON["isRealEstate"] != nil && e.DetailJSON["isRealEstate"].(bool) {
			// else if it is Real Estate, add the cost basis to the transaction
			trn.CostBasis = trn.Amount
		}

		trn.addEvent(networth.EventCalculationEntry{
			Entry:          "Debt",
			Editable:       false,
			CapitalAccount: trn.CapitalAccount,
			CostBasis:      trn.CostBasis,
		})
	}

	if trn.ExecuteType == networth.ETPreferredReturn {
		trn.CapitalAccount = -1.00 * trn.Amount
		trn.CostBasis = -1.00 * trn.Amount
		trn.addEvent(networth.EventCalculationEntry{
			Entry:          "Preferred Return",
			Editable:       false,
			CapitalAccount: trn.CapitalAccount,
			CostBasis:      trn.CostBasis,
		})
	}

	if trn.ExecuteType == networth.ETReturnOfCapital || trn.ExecuteType == networth.ETExternalReturnOfCapital {
		trn.CapitalAccount = -1.00 * trn.Amount
		trn.CostBasis = -1.00 * trn.Amount
		trn.addEvent(networth.EventCalculationEntry{
			Entry:          "Return of Capital",
			Editable:       false,
			CapitalAccount: trn.CapitalAccount,
			CostBasis:      trn.CostBasis,
		})
	}

	if trn.ExecuteType == networth.ETTaxDistribution || trn.ExecuteType == networth.ETExternalTaxDistribution {
		trn.CapitalAccount = -1.00 * trn.Amount
		trn.CostBasis = -1.00 * trn.Amount
		trn.addEvent(networth.EventCalculationEntry{
			Entry:          "Tax Distribution",
			Editable:       false,
			CapitalAccount: trn.CapitalAccount,
			CostBasis:      trn.CostBasis,
		})
	}

	if trn.ExecuteType == networth.ETFundSponsorPromote || trn.ExecuteType == networth.ETExternalFundSponsorPromote {
		trn.CapitalAccount = -1.00 * trn.Amount
		trn.CostBasis = -1.00 * trn.Amount
		trn.addEvent(networth.EventCalculationEntry{
			Entry:          "Profit Distribution",
			Editable:       false,
			CapitalAccount: trn.CapitalAccount,
			CostBasis:      trn.CostBasis,
		})
	}

	if (trn.ExecuteType == networth.ETCashTransfer || trn.ExecuteType == networth.ETExternalCashTransfer) && trn.WaterfallID != "" {
		element := networth.FindWaterfallElement(trn.WaterfallID)
		capitalAccount := util.RoundMoney(trn.CapitalAccount * element.CapitalAccount)
		costBasis := util.RoundMoney(trn.CostBasis * element.CostBasis)
		if costBasis != 0.00 || capitalAccount != 0.00 {
			trn.CapitalAccount = -1.00 * trn.Amount
			trn.CostBasis = -1.00 * trn.Amount
			trn.addEvent(networth.EventCalculationEntry{
				Entry:          element.Name,
				Editable:       false,
				CapitalAccount: trn.CapitalAccount,
				CostBasis:      trn.CostBasis,
			})
		}
	}

	/*
		if trn.ExecuteType == networth.ETAdjustment {
			if meta.Adjustment.CapitalAccount != 0.00 || meta.Adjustment.CostBasis != 0.00 {
				trn.CapitalAccount = meta.Adjustment.CapitalAccount * math.Abs(trn.Amount)
				trn.CostBasis = meta.Adjustment.CostBasis * math.Abs(trn.Amount)
				trn.addEvent(networth.EventCalculationEntry{
					Entry:          meta.Adjustment.Name,
					Editable:       false,
					CapitalAccount: trn.CapitalAccount,
					CostBasis:      trn.CostBasis,
				})
			}
		}
	*/

	*transaction = trn
}

func (transaction *Transaction) addEvent(evt networth.EventCalculationEntry) {
	trn := *transaction
	trn.EventCalculations = append(trn.EventCalculations, evt)
	*transaction = trn
}
