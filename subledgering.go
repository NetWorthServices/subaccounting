package subledger

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"git.aax.dev/agora-altx/apiarcade/cache"
	"git.aax.dev/agora-altx/models-go/networth"
	"git.aax.dev/agora-altx/utils-go/database"
	"git.aax.dev/agora-altx/utils-go/util"
	"git.aax.dev/agora-altx/utils-go/util/logging"
	"github.com/go-pg/pg"
)

type dbLog struct{}

func (d dbLog) BeforeQuery(q *pg.QueryEvent) {}
func (d dbLog) AfterQuery(q *pg.QueryEvent) {
	fmt.Println(q.FormattedQuery())
}

func redisKey(act string) string {
	return "subledger:" + act
}

func getFromCache(accountID string) (Subledger, bool) {
	r := cache.SetupRedis()
	defer r.Close()
	sl := Subledger{}

	if str, err := r.Get(redisKey(accountID)).Result(); err == nil {
		e := json.Unmarshal([]byte(str), &sl)
		return sl, e == nil
	}
	return Subledger{}, false
}

// ReInit reinitalizes this subledger
func ReInit(accountID string) Subledger {
	ClearCache(accountID)
	return Init(accountID)
}

// ClearCache forces the removal of the account from its internal cache so it can get re-ran
func ClearCache(accountID string) {
	r := cache.SetupRedis()
	defer r.Close()
	r.Del(redisKey(accountID))
}

// Init initalizes or retrieves a subledger
func Init(accountID string) (sl Subledger) {

	if newSL, ok := getFromCache(accountID); ok {
		sl = newSL
		return
	}

	type feeTrackingMap map[string]float64
	feeTracking := feeTrackingMap{}

	theAccount := networth.Account{}
	theAccount.Find(accountID)
	theAccount.Populate()

	networth.ClearCaches("IRR:" + theAccount.ID + ":*")

	sl.Accounts = make(map[string]networth.Account)
	sl.AccountID = accountID
	//sl.Balances = make(map[string]float64)

	db := &database.CQ{}
	defer db.Close()

	db.Init()
	db.UserType = database.DATABASE_USER_TYPE_READ_AND_WRITE_ONLY
	db.EnableCache(false)

	type getAccountActivity struct {
		tableName struct{} `sql:"app.activity"`
		ID        string   `sql:"id"`
		Thread    string   `sql:"thread"`
	}

	var accountActivities []networth.Activity

	err := db.Model(&accountActivities).Where("thread::text LIKE ?", "%"+accountID+"%").Where(`"status" > 0`).Select()
	if err != nil {
		logging.Log(logging.Message{
			Level: logging.Error,
			Text:  err,
		})
		return
	}

	asset := networth.Asset{}
	pricePerUnit := 0.00

	for idx := 0; idx < len(accountActivities); idx++ {
		act := accountActivities[idx]
		act.Populate()
		invokedByList := []string{}

		tData := act.ThreadJSON

		for k := 0; k < len(tData); k++ {
			if tData[k].Envelope.ExecuteType != networth.ETDefault {
				if tData[k].Envelope.InvokedBy != "" {
					invokedByList = append(invokedByList, tData[k].Envelope.InvokedBy)
				}

				if _, ok := feeTracking[accountActivities[idx].ID]; !ok {
					feeTracking[accountActivities[idx].ID] = util.Float64FromString(tData[k].Envelope.TotalFee)
				}

				if asset.ID != tData[k].Envelope.AssetID {
					asset.Find(tData[k].Envelope.AssetID)
					asset.Populate()
				}

				invClass := tData[k].Envelope.InvestmentClass

				// if theAccount.DetailJSON["investmentType"] != nil {
				it := invClass.InvestmentType
				if asset.DetailJSON["investmentClasses"] != nil {
					for _, ty := range asset.DetailJSON["investmentClasses"].([]interface{}) {
						theType := ty.(map[string]interface{})
						if theType["investmentType"].(string) == it {
							pricePerUnit = theType["unitPrice"].(float64)
						}
					}
				} else {
					pricePerUnit = util.Float64FromString(invClass.UnitPrice)
				}
				// } else {
				// 	pricePerUnit = util.Float64FromString(asset.DetailJSON.String("unitPrice"))
				// }

				amount := util.Float64FromString(tData[k].Envelope.BankAmount)
				if tData[k].Envelope.BankAmount == "" {
					amount = util.Float64FromString(tData[k].Envelope.Amount)
				}
				fee := feeTracking[accountActivities[idx].ID]
				feeTracking[accountActivities[idx].ID] = 0.00

				timestamp := parseDate(tData[k].Created)

				tData[k].Envelope.FromAccountID, tData[k].Envelope.ToAccountID = checkAccountIDS(tData[k].ID, tData[k].Envelope)

				t, ceid, desc := getTypeCounterEntityAndDesc(theAccount, tData[k].Envelope)

				if t == "" {
					continue
				}
				units := 0.00
				if pricePerUnit > 0.00 {
					units = (amount - fee) / pricePerUnit
				}

				fundAccount := tData[k].Envelope.NonMonetaryAccountID
				if fundAccount == "" {
					tmpAct := networth.Account{
						IDEntity:          tData[k].Envelope.Context.Investor,
						IDCustodialEntity: tData[k].Envelope.Context.Fund,
						Type:              networth.ACTInvestment,
					}
					tmpAct.Search()
					if tmpAct.ID != "" {
						fundAccount = tmpAct.ID
					}
				}

				trn := Transaction{
					ID:                                  tData[k].ID,
					To:                                  getCorrectAccountID(tData[k].Envelope.ToAccountID, tData[k].Envelope.ToEntityID, tData[k].Envelope.ToAccountDetail),
					From:                                getCorrectAccountID(tData[k].Envelope.FromAccountID, fallback(tData[k].Envelope.FromEntityID, tData[k].Envelope.From), tData[k].Envelope.FromAccountDetail),
					FundAct:                             fundAccount,
					ActivityID:                          accountActivities[idx].ID,
					ExecuteType:                         tData[k].Envelope.ExecuteType,
					Type:                                t,
					IdentifyCapitalType:                 tData[k].Envelope.IdentifyCapitalType,
					CounterEntityID:                     ceid,
					TransactionRequestDate:              tData[k].Envelope.TransactionRequestDate,
					TransactionDate:                     tData[k].Created.Format(util.DateFormat("m/d/Y")),
					TransactionType:                     tData[k].Envelope.TransactionType,
					BankTransactionDate:                 tData[k].Envelope.BankTransactionDate,
					PayDate:                             "",
					AssetID:                             tData[k].Envelope.AssetID,
					Description:                         desc,
					BankTransactionID:                   tData[k].Envelope.BankTransactionID,
					BankMemo:                            tData[k].Envelope.BankMemo,
					TotalAmount:                         amount,
					Amount:                              amount - fee,
					Fee:                                 fee,
					Units:                               units,
					BankAmount:                          util.Float64FromString(tData[k].Envelope.BankAmount),
					CostBasis:                           0.00,
					Timestamp:                           tData[k].Created,
					Time:                                timestamp,
					InvestmentClass:                     invClass,
					FundSponsorOwnershipDetermination:   util.ToString(tData[k].Envelope.FundSponsorInvestment["fundSponsorOwnershipDetermination"]),
					InitialCapitalContributionInclusion: strings.ToUpper(util.ToString(tData[k].Envelope.FundSponsorInvestment["initialCapitalContributionInclusion"])) == "TRUE",
					Documents:                           tData[k].Envelope.GatherDocuments(),
					ApprovedBy:                          tData[k].Envelope.ApprovedBy,
					CapitalStack:                        tData[k].Envelope.CapitalStack,
					WaterfallID:                         tData[k].Envelope.WaterfallID,
					Guarantors:                          tData[k].Envelope.Debt.Guarantors,
				}
				if pricePerUnit > 0 {
					trn.Units = (amount - fee) / pricePerUnit
				}
				if util.Float64FromString(util.ToString(tData[k].Envelope.FundSponsorInvestment["initialCapitalContribution"])) > 0.00 {
					trn.InitialCapitalContribution = util.Float64FromString(util.ToString(tData[k].Envelope.FundSponsorInvestment["initialCapitalContribution"]))
				}
				if util.Float64FromString(util.ToString(tData[k].Envelope.FundSponsorInvestment["fundSponsorNonCashContribution"])) > 0.00 {
					trn.FundSponsorNonCashContribution = util.Float64FromString(util.ToString(tData[k].Envelope.FundSponsorInvestment["fundSponsorNonCashContribution"]))
				}
				if util.Float64FromString(util.ToString(tData[k].Envelope.FundSponsorInvestment["fundSponsorOwnershipPercentage"])) > 0.00 {
					trn.FundSponsorOwnershipPercentage = util.Float64FromString(util.ToString(tData[k].Envelope.FundSponsorInvestment["fundSponsorOwnershipPercentage"]))
				}

				if theAccount.Type == networth.ACTInvestment {
					trn.processFundEvents(tData[k].Envelope)
				}

				if trn.Type == networth.TTCreditDebit {
					// We do this because it is both a credit and a debit transaction.
					trn.Amount = util.Float64FromString(tData[k].Envelope.Amount)
					trn.Type = networth.TTCredit
					sl.TransactionsCalc = append(sl.TransactionsCalc, trn)
					trn.Type = networth.TTDebit
					sl.TransactionsCalc = append(sl.TransactionsCalc, trn)
				} else {
					sl.TransactionsCalc = append(sl.TransactionsCalc, trn)
				}
				// We do our IRR add here
				addToIRR(theAccount.ID, trn)
			}

			if sl.Accounts[tData[k].Envelope.ToAccountID].ID == "" {
				sl.Accounts[tData[k].Envelope.ToAccountID] = networth.Account{
					Balance: 0,
				}
			}

		}
	}

	sl.aggregateSequentially()
	sl.saveToCache()

	return
}

func addToIRR(accountID string, trn Transaction) {
	acct := networth.FindAccount(accountID)
	ent := networth.FindEntity(acct.IDEntity)
	ceid := trn.CounterEntityID
	ce := networth.FindEntity(ceid)
	ts := trn.Timestamp
	amount := trn.Amount

	fyEndMonth := 0
	fyEndDay := 0
	startDate := time.Time{}

	entFY := ent.DetailJSON.String("fiscalYear")
	ceFY := ce.DetailJSON.String("fiscalYear")

	if ent.Type == networth.MTFund && ce.Type == networth.MTBusiness && (trn.ExecuteType != networth.ETNonFundEquity && trn.ExecuteType != networth.ETExternalNonFundEquity) {
		// Handling Fund to Business Investments
		if len(entFY) > 0 {
			fyEndMonth = util.StringToInt(entFY[0:2])
			fyEndDay = util.StringToInt(entFY[2:])
		} else {
			fyEndMonth = 12
			fyEndDay = 31
		}
		startDate, _ = time.Parse(util.DateFormat("Y-m-d"), ent.DetailJSON.String("incorporationDate"))
	} else if ent.Type == networth.MTBusiness && (ceid == "" || ceid == ent.ID) {
		// Handling Business to Project Investments
		if len(entFY) > 0 {
			fyEndMonth = util.StringToInt(entFY[0:2])
			fyEndDay = util.StringToInt(entFY[2:])
		} else {
			fyEndMonth = 12
			fyEndDay = 31
		}
		startDate, _ = time.Parse(util.DateFormat("Y-m-d"), ent.DetailJSON.String("incorporationDate"))
	} else if (trn.ExecuteType == networth.ETSubscription || trn.ExecuteType == networth.ETExternalSubscription || trn.ExecuteType == networth.ETSale) && ce.Type == networth.MTFund {
		// Handling what the Investor IRR would be
		if len(ceFY) > 0 {
			fyEndMonth = util.StringToInt(ceFY[0:2])
			fyEndDay = util.StringToInt(ceFY[2:])
		} else {
			fyEndMonth = 12
			fyEndDay = 31
		}
		startDate, _ = time.Parse(util.DateFormat("Y-m-d"), ce.DetailJSON.String("incorporationDate"))
	} else if (trn.ExecuteType == networth.ETNonFundEquity || trn.ExecuteType == networth.ETExternalNonFundEquity) && ce.Type == networth.MTBusiness {
		// Handling Direct Investment to the Business
		if len(ceFY) > 0 {
			fyEndMonth = util.StringToInt(ceFY[0:2])
			fyEndDay = util.StringToInt(ceFY[2:])
		} else {
			fyEndMonth = 12
			fyEndDay = 31
		}
		startDate, _ = time.Parse(util.DateFormat("Y-m-d"), ce.DetailJSON.String("incorporationDate"))
	} else {
		// Get the hell out of here, it isn't an IRR transaction
		return
	}

	// Determine the FiscalYearQuarter for the transaction.

	newStartDate := time.Date(startDate.Year(), time.Month(fyEndMonth), fyEndDay+1, 0, 0, 0, 0, time.UTC)
	if newStartDate.After(startDate) {
		for notDone := newStartDate.After(startDate); notDone; notDone = newStartDate.After(startDate) {
			newStartDate = newStartDate.AddDate(0, -3, 0)
		}
	} else {
		for notDone := newStartDate.AddDate(0, 3, 0).Before(startDate); notDone; notDone = newStartDate.AddDate(0, 3, 0).Before(startDate) {
			newStartDate = newStartDate.AddDate(0, 3, 0)
		}
	}

	yr, mo, da, _, _, _ := util.DateDiff(newStartDate, ts)
	if da > 0 {
		mo += 1
	}
	Q := math.Ceil(float64(mo) / 4.00)
	yq := fmt.Sprintf("Y%vQ%v", yr+1, Q)

	// Get the cache of the IRR
	cacheName := fmt.Sprintf("IRR:%v:%v", accountID, ceid)
	caches := networth.FindCaches(cacheName)

	if len(caches) == 0 {
		caches = append(caches, networth.Cache{})
	}
	cache := caches[0]

	data := util.JSONObject{}
	json.Unmarshal(cache.JSON(), &data)

	// Add
	if data[yq] == nil {
		data[yq] = amount
	} else {
		data[yq] = data[yq].(float64) + amount
	}

	// Save to Cache
	raw, _ := json.Marshal(data)
	cache.Value = util.ParseString(raw)
	cache.Save()
}

func fallback(this, that string) string {
	if this != "" {
		return this
	}
	return that
}

func getCorrectAccountID(accountID, entityID string, details networth.AccountDetail) string {
	if accountID != "" {
		return accountID
	}

	ent := networth.Entity{}
	ent.Find(entityID)
	fromObj := details.ToJSONObject()
	obj := util.JSONObject{
		"accountNumber": fromObj["accountNumber"],
		"routingNumber": fromObj["routingNumber"],
	}
	acts := ent.GetMyAccounts(&obj)

	if len(acts) > 0 {
		return acts[0].ID
	}

	return ""
}

func getTypeCounterEntityAndDesc(account networth.Account, env networth.ActivityMetaData) (execType, ceid, desc string) {

	switch env.ExecuteType {
	case networth.ETCashTransfer,
		networth.ETExternalCashTransfer,
		networth.ETCashReserves,
		networth.ETOperatingExpense,
		networth.ETOrganizationalExpense,
		networth.ETTaxDistribution,
		networth.ETInvestorPreferred,
		networth.ETFundSponsorPromote,
		networth.ETReturnOfCapital,
		networth.ETFundSponsorManagementFee,
		networth.ETExternalCashReserves,
		networth.ETExternalOperatingExpense,
		networth.ETExternalOrganizationalExpense,
		networth.ETExternalTaxDistribution,
		networth.ETExternalInvestorPreferred,
		networth.ETExternalFundSponsorPromote,
		networth.ETExternalReturnOfCapital,
		networth.ETExternalFundSponsorManagementFee:
		if env.ToAccountID == account.ID {
			execType = string(networth.TTDebit)
			ceid = env.FromEntityID
			if ceid != "" {
				acct := networth.Account{}
				acct.Find(env.FromAccountID)
				desc = fmt.Sprintf("Fund Transfer from %s", acct.DetailJSON["name"])
				if acct.Type == networth.ACTExternal || acct.Type == networth.ACTHistorical {
					desc = "Cash Transfer from External Account"
				}
			} else {
				desc = fmt.Sprintf("Fund Transfer from %s", env.FromAccountDetail.Name)
				if env.ExecuteType == networth.ETHistorical {
					acct := networth.Account{}
					acct.Find(env.FromAccountID)
					desc = fmt.Sprintf("Fund Transfer from %s", acct.DetailJSON["name"])
				}
			}
		} else {
			execType = string(networth.TTCredit)
			ceid = env.ToEntityID
			if ceid != "" {
				acct := networth.Account{}
				acct.Find(env.ToAccountID)
				desc = fmt.Sprintf("Fund Transfer to %s", acct.DetailJSON["name"])
				if acct.Type == networth.ACTExternal || acct.Type == networth.ACTHistorical {
					desc = "Cash Transfer to External Account"
				}
			} else {
				desc = fmt.Sprintf("Fund Transfer to %s", env.ToAccountDetail.Name)
				if env.ExecuteType == networth.ETHistorical {
					acct := networth.Account{}
					acct.Find(env.ToAccountID)
					desc = fmt.Sprintf("Fund Transfer to %s", acct.DetailJSON["name"])
				}
			}
		}
		if env.ToAccountID == env.FromAccountID {
			execType = string(networth.TTCreditDebit)
		}
	case networth.ETHistorical,
		networth.ETTaxCredit,
		networth.ETExternalTaxCredit,
		networth.ETNonFundEquity,
		networth.ETExternalNonFundEquity,
		networth.ETDebt,
		networth.ETExternalDebt:
		if env.ToAccountID == account.ID {
			execType = string(networth.TTDebit)
			ceid = env.FromEntityID
			if ceid != "" {
				acct := networth.Account{}
				acct.Find(env.FromAccountID)
				desc = fmt.Sprintf("Fund Transfer from %s", acct.DetailJSON["name"])
				if acct.Type == networth.ACTExternal || acct.Type == networth.ACTHistorical {
					desc = "Cash Transfer from External Account"
				}
			} else {
				desc = fmt.Sprintf("Fund Transfer from %s", env.FromAccountDetail.Name)
				if env.ExecuteType == networth.ETHistorical {
					acct := networth.Account{}
					acct.Find(env.FromAccountID)
					desc = fmt.Sprintf("Fund Transfer from %s", acct.DetailJSON["name"])
				}
			}
		} else {
			execType = string(networth.TTCredit)
			ceid = env.ToEntityID
			if ceid != "" {
				acct := networth.Account{}
				acct.Find(env.ToAccountID)
				desc = fmt.Sprintf("Fund Transfer to %s", acct.DetailJSON["name"])
				if acct.Type == networth.ACTExternal || acct.Type == networth.ACTHistorical {
					desc = "Cash Transfer to External Account"
				}
			} else {
				desc = fmt.Sprintf("Fund Transfer to %s", env.ToAccountDetail.Name)
				if env.ExecuteType == networth.ETHistorical {
					acct := networth.Account{}
					acct.Find(env.ToAccountID)
					desc = fmt.Sprintf("Fund Transfer to %s", acct.DetailJSON["name"])
				}
			}
		}
	case networth.ETSubscription, networth.ETExternalSubscription:
		fundAcct := networth.Account{}
		eid := env.Context.Fund
		if eid == "" {
			eid = env.Context.Entity
		}
		fundAcct.FindForEntityOfType(eid, networth.ACTEscrow)
		fundAcct.Populate()

		asset := networth.Asset{
			ID: env.AssetID,
		}
		asset.Find()

		if account.Type == networth.ACTEscrow {
			execType = networth.TTDebit
		} else {
			execType = networth.TTPurchase
		}
		if asset.DetailJSON["name"] != nil {
			desc = fmt.Sprintf("Subscription to %s", asset.DetailJSON["name"].(string))
		} else {
			desc = fmt.Sprintf("Subscription to %s", asset.Name)
		}
		if env.Conversion.ToAsset == asset.ID {
			fromAsset := networth.FindAsset(env.Conversion.FromAsset)

			if fromAsset.DetailJSON["name"] != nil {
				desc = fmt.Sprintf("%s (Converted from %s)", desc, fromAsset.DetailJSON["name"].(string))
			} else {
				desc = fmt.Sprintf("%s (Converted from %s)", desc, fromAsset.Name)
			}
		}
	case networth.ETSale:
		execType = networth.TTSale

		asset := networth.Asset{
			ID: env.AssetID,
		}
		asset.Find()

		if asset.DetailJSON["name"] != nil {
			desc = fmt.Sprintf("Selling of %s", asset.DetailJSON["name"].(string))
		} else {
			desc = fmt.Sprintf("Selling of %s", asset.Name)
		}
	case networth.ETConversion:
		execType = networth.TTConversion

		asset := networth.FindAsset(env.Conversion.ToAsset)

		if asset.DetailJSON["name"] != nil {
			desc = fmt.Sprintf("Converting to %s", asset.DetailJSON["name"].(string))
		} else {
			desc = fmt.Sprintf("Converting to %s", asset.Name)
		}
	case networth.ETPreferredReturn:
		execType = networth.TTDistribution
	case networth.ETSponsor:
		if account.Type == networth.ACTSponsor {
			execType = networth.TTSponsor
		} else {
			execType = ""
		}
	}

	return
}

func checkAccountIDS(id string, meta networth.ActivityMetaData) (fromID, toID string) {
	if meta.FromAccountID != "" {
		fromID = meta.FromAccountID
	} else if meta.NonMonetaryAccountID != "" {
		fromID = meta.NonMonetaryAccountID
	} else {
		// Use ToEntityID because that is the known entity
		entID := meta.Context.Entity
		if entID == "" {
			entID = meta.Context.Investor
		}

		ent := networth.Entity{}
		ent.Find(entID)
		ent.Populate()
		criteria := util.JSONObject{"accountNumber": meta.FromAccountDetail.AccountNumber, "routingNumber": meta.FromAccountDetail.RoutingNumber}
		accts := ent.GetMyAccounts(&criteria)

		if len(accts) > 0 {
			fromID = accts[0].ID
		} else {
			fmt.Printf("ID: %v\nFrom Ent: %v   Crit: %v\n", id, entID, criteria)
			fromID = ""
		}
	}
	if meta.ToAccountID != "" {
		toID = meta.ToAccountID
	} else {
		// Use FromEntityID because that is the known entity
		entID := meta.Context.Entity
		if entID == "" {
			entID = meta.Context.Investor
		}

		ent := networth.Entity{}
		ent.Find(entID)
		ent.Populate()
		criteria := util.JSONObject{"accountNumber": meta.ToAccountDetail.AccountNumber, "routingNumber": meta.ToAccountDetail.RoutingNumber}
		accts := ent.GetMyAccounts(&criteria)

		if len(accts) > 0 {
			toID = accts[0].ID
		} else {
			fmt.Printf("ID: %v\nTo Ent: %v   Crit: %v\n", id, entID, criteria)
			toID = ""
		}
	}

	return
}

// aggregateSequentially sorts transactions sequentially by TimeInt
func (payload *Subledger) saveToCache() {
	pl := *payload

	r := cache.SetupRedis()
	defer r.Close()
	if raw, err := json.Marshal(pl); err == nil {
		r.Set("subledger:"+pl.AccountID, string(raw), 0)
	}
}

// aggregateSequentially sorts transactions sequentially by TimeInt
func (payload *Subledger) aggregateSequentially() {
	pl := *payload

	// Sort pl.TransactionsCalc by converted TimeInt
	sort.Slice(pl.TransactionsCalc, func(i, j int) bool {
		return pl.TransactionsCalc[i].Timestamp.Before(pl.TransactionsCalc[j].Timestamp)
	})

	// Adjust transaction balance application for subsequent transactions
	pl.aggregateTransactionNet()

	*payload = pl
}

func (payload *Subledger) aggregateTransactionNet() {
	pl := *payload
	// Having to do this so force copy by value
	for idx := 0; idx < len(pl.TransactionsCalc); idx++ {
		pl.TransactionsNet = append(pl.TransactionsNet, pl.TransactionsCalc[idx].clone())
	}

	pl.GrandTotal = 0.00

	for i := 0; i < len(pl.TransactionsCalc); i++ {
		trn := pl.TransactionsCalc[i]
		act := networth.Account{
			ID: pl.AccountID,
		}
		act.Find()

		//currentTransaction = pl.TransactionsNet[i].Fr
		if trn.From == pl.AccountID && act.Type != networth.ACTInvestment {
			// This is the FROM account
			// Calculate where the money came from and attach it to the transaction
			pl.GrandTotal -= trn.Amount

			asset := networth.Asset{}
			asset.Find(pl.AssetID)
			asset.Populate()

			fund := networth.Entity{}
			fund.Find(asset.IDEntity)
			fund.Populate()

			ty := ""
			if fund.DetailJSON["subaccountingMethod"] != nil {
				ty = fund.DetailJSON["subaccountingMethod"].(string)
			}

			pl.TransactionsCalc[i].Subledger = pl.Execute(Transfer{
				Amount:    trn.Amount,
				Type:      ty,
				Timestamp: trn.Timestamp,
			})
		} else {
			// This is the TO account
			// Take the From transaction and add it to the pool
			pl.GrandTotal += trn.Amount
			if trn.ExecuteType == networth.ETSubscription || trn.ExecuteType == networth.ETExternalSubscription {
				pl.TransactionsCalc[i].Subledger = append(pl.TransactionsCalc[i].Subledger, pl.addInvestment(trn))
				pl.AssetID = trn.AssetID
			} else {
				fromAccount := Init(trn.From)
				fTrn := fromAccount.findTransaction(trn.ID)
				pl.transferInvestment(fTrn.Subledger)
				pl.TransactionsCalc[i].Subledger = fTrn.Subledger
				if pl.AssetID == "" {
					pl.AssetID = fromAccount.AssetID
				}
			}
		}
	}

	*payload = pl
}

func (payload *Subledger) addInvestment(trans Transaction) networth.Investor {
	pl := *payload
	inv := networth.Investor{
		PathchainID:     trans.ID,
		InvestorAccount: trans.FundAct,
		Amount:          trans.Amount, // Convert timestamp to int and push to pl.TimeInt
		Timestamp:       trans.Timestamp,
	}
	pl.Investments = append(pl.Investments, inv)
	*payload = pl
	return inv
}

func (payload *Subledger) transferInvestment(trans []networth.Investor) {
	pl := *payload
	pl.Investments = append(pl.Investments, trans...)
	*payload = pl
}

func (payload *Subledger) findTransaction(id string) Transaction {
	pl := *payload
	for _, t := range pl.TransactionsCalc {
		if t.ID == id {
			return t
		}
	}
	return Transaction{} // How the hell did this happen!?
}

func (payload *Subledger) aggregateTransactionSubtract(amount float64) {
	pl := *payload
	subtraction := 0.0
	deficit := amount

	for i := 0; i < len(pl.TransactionsNet); i++ {

		if pl.TransactionsCalc[i].To == pl.AccountID {
			amt := pl.TransactionsNet[i].Amount

			fmt.Printf("%v - %v = %v\n", pl.TransactionsNet[i].Amount, amount, amt-amount)

			if (amt - deficit) > 0 {
				subtraction = deficit
				deficit = 0
			} else {
				subtraction = amt
				deficit = deficit - subtraction
			}

			pl.TransactionsNet[i].Amount = amt - subtraction

			if deficit <= 0 {
				break
			}
		}
	}

	*payload = pl
}

// ParseDate returns RFC3339Nano timestamp as TimeUnits
func parseDate(t time.Time) networth.TimeUnits {
	u := networth.TimeUnits{}

	u.Year = util.StringToInt(t.Format("2006"))
	u.Month = util.StringToInt(t.Format("01"))
	u.Day = util.StringToInt(t.Format("02"))
	u.Hour = util.StringToInt(t.Format("15"))
	u.Minute = util.StringToInt(t.Format("04"))
	u.Second = util.StringToInt(t.Format("05"))

	return u
}

// ParseDateCode converts TimeUnits to timestamp as int
func parseDateCode(units TimeUnits) int {
	y := fmt.Sprintf(`%v`, units.Year)
	m := zerofy(units.Month)
	d := zerofy(units.Day)
	h := zerofy(units.Hour)
	n := zerofy(units.Minute)
	s := zerofy(units.Second)

	code := fmt.Sprintf(`%v%s%s%s%s%s`, y, m, d, h, n, s)

	return util.StringToInt(code)
}

func zerofy(num int) string {
	val := fmt.Sprintf(`%v`, num)

	if num < 10 {
		val = fmt.Sprintf(`0%v`, num)
	}
	return val
}
