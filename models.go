package subaccounting

import (
	"sort"
	"time"

	"git.aax.dev/agora-altx/models-go/networth"
	"github.com/jinzhu/copier"
)

// Subledger main subledgering payload struct
type Subledger struct {
	AccountID        string                      `json:"-"`
	Accounts         map[string]networth.Account `json:"-"`
	GrandTotal       float64                     `json:"grandTotal"`
	TransactionsCalc TransactionList             `json:"transactions"`
	TransactionsNet  TransactionList             `json:"-"`
	Investments      []networth.Investor         `json:"investments"`
	AssetID          string                      `json:"assetID"`
	//Balances     map[string]Account
}

type TransactionSequential struct {
	RootTransaction, From               string
	Units, Amount, Subtraction, Deficit float64
	TimeInt                             int
}

type TransactionList []Transaction

// Transaction Payload.Transaction struct
type Transaction networth.IntervalTransaction

func (t *Transaction) clone() Transaction {
	tmp := Transaction{}
	copier.Copy(&tmp, &t)

	return tmp
}

// FilterBy the function structure for filtering out transactions
type FilterBy func(t Transaction) bool

// Filter looks for which Transactions to show
func (tl *TransactionList) Filter(fcn FilterBy) (newList TransactionList) {
	for _, t := range *tl {
		if fcn(t) {
			newList = append(newList, t.clone())
		}
	}
	return
}

// Sort sorts the transactions by Timestamp date
func (tl *TransactionList) Sort() {
	list := *tl
	sort.Slice(list[:], func(i, j int) bool {
		return list[i].Timestamp.Before(list[j].Timestamp)
	})
	*tl = list
}

// GetAllForExecuteAndTransactionTypes Gets all transactions by that ExecuteType and Transaction Type
func (tl *TransactionList) GetAllForExecuteAndTransactionTypes(et int, ty string) TransactionList {
	return tl.Filter(func(t Transaction) bool {
		return t.ExecuteType == et && t.Type == ty
	})
}

// GetAllForExecuteAndTransactionTypesWithCounterEntity Gets all transactions by that ExecuteType and Transaction Type of the Counter Entity
func (tl *TransactionList) GetAllForExecuteAndTransactionTypesWithCounterEntity(et int, ty string, ceid string) TransactionList {
	return tl.Filter(func(t Transaction) bool {
		return t.ExecuteType == et && t.Type == ty && t.CounterEntityID == ceid
	})
}

// GetAllForWaterfallAndTransactionTypes Gets all transactions by that ExecuteType and Transaction Type
func (tl *TransactionList) GetAllForWaterfallAndTransactionTypes(wid, ty string) TransactionList {
	return tl.Filter(func(t Transaction) bool {
		return t.WaterfallID == wid && t.Type == ty
	})
}

// GetAllForWaterfallAndTransactionTypesWithCounterEntity Gets all transactions by that ExecuteType and Transaction Type of the Counter Entity
func (tl *TransactionList) GetAllForWaterfallAndTransactionTypesWithCounterEntity(wid, ty, ceid string) TransactionList {
	return tl.Filter(func(t Transaction) bool {
		return t.WaterfallID == wid && t.Type == ty && t.CounterEntityID == ceid
	})
}

// TimeUnits Subledger.Transaction.TimeUnits struct
type TimeUnits networth.TimeUnits

// Stack of transactions
type Stack map[int]networth.ActivityMetaData

// Transfer transaction type
type Transfer struct {
	Type          string
	Amount, Units float64
	Timestamp     time.Time
}
