// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package satellitedb

import (
	"context"
	"math/big"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeebo/errs"

	"storj.io/common/uuid"
	"storj.io/storj/satellite/payments/coinpayments"
	"storj.io/storj/satellite/payments/monetary"
	"storj.io/storj/satellite/payments/stripecoinpayments"
	"storj.io/storj/satellite/satellitedb/dbx"
)

// ensure that coinpaymentsTransactions implements stripecoinpayments.TransactionsDB.
var _ stripecoinpayments.TransactionsDB = (*coinPaymentsTransactions)(nil)

// applyBalanceIntentState defines states of the apply balance intents.
type applyBalanceIntentState int

const (
	// apply balance intent waits to be applied.
	applyBalanceIntentStateUnapplied applyBalanceIntentState = 0
	// transaction which balance intent points to has been consumed.
	applyBalanceIntentStateConsumed applyBalanceIntentState = 1
)

// Int returns intent state as int.
func (intent applyBalanceIntentState) Int() int {
	return int(intent)
}

// coinPaymentsTransactions is CoinPayments transactions DB.
//
// architecture: Database
type coinPaymentsTransactions struct {
	db *satelliteDB
}

// Insert inserts new coinpayments transaction into DB.
func (db *coinPaymentsTransactions) Insert(ctx context.Context, tx stripecoinpayments.Transaction) (_ *stripecoinpayments.Transaction, err error) {
	defer mon.Task()(&ctx)(&err)

	amount, err := tx.Amount.AsBigFloat().GobEncode()
	if err != nil {
		return nil, errs.Wrap(err)
	}
	received, err := tx.Received.AsBigFloat().GobEncode()
	if err != nil {
		return nil, errs.Wrap(err)
	}

	dbxCPTX, err := db.db.Create_CoinpaymentsTransaction(ctx,
		dbx.CoinpaymentsTransaction_Id(tx.ID.String()),
		dbx.CoinpaymentsTransaction_UserId(tx.AccountID[:]),
		dbx.CoinpaymentsTransaction_Address(tx.Address),
		dbx.CoinpaymentsTransaction_Amount(amount),
		dbx.CoinpaymentsTransaction_Received(received),
		dbx.CoinpaymentsTransaction_Status(tx.Status.Int()),
		dbx.CoinpaymentsTransaction_Key(tx.Key),
		dbx.CoinpaymentsTransaction_Timeout(int(tx.Timeout.Seconds())),
	)
	if err != nil {
		return nil, err
	}

	return fromDBXCoinpaymentsTransaction(dbxCPTX)
}

// Update updates status and received for set of transactions.
func (db *coinPaymentsTransactions) Update(ctx context.Context, updates []stripecoinpayments.TransactionUpdate, applies coinpayments.TransactionIDList) (err error) {
	defer mon.Task()(&ctx)(&err)

	if len(updates) == 0 {
		return nil
	}

	return db.db.WithTx(ctx, func(ctx context.Context, tx *dbx.Tx) error {
		for _, update := range updates {
			received, err := update.Received.AsBigFloat().GobEncode()
			if err != nil {
				return errs.Wrap(err)
			}

			_, err = tx.Update_CoinpaymentsTransaction_By_Id(ctx,
				dbx.CoinpaymentsTransaction_Id(update.TransactionID.String()),
				dbx.CoinpaymentsTransaction_Update_Fields{
					Received: dbx.CoinpaymentsTransaction_Received(received),
					Status:   dbx.CoinpaymentsTransaction_Status(update.Status.Int()),
				},
			)
			if err != nil {
				return err
			}
		}

		for _, txID := range applies {
			query := db.db.Rebind(`INSERT INTO stripecoinpayments_apply_balance_intents ( tx_id, state, created_at )
			VALUES ( ?, ?, ? ) ON CONFLICT DO NOTHING`)
			_, err = tx.Tx.ExecContext(ctx, query, txID.String(), applyBalanceIntentStateUnapplied.Int(), db.db.Hooks.Now().UTC())
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// Consume marks transaction as consumed, so it won't participate in apply account balance loop.
func (db *coinPaymentsTransactions) Consume(ctx context.Context, id coinpayments.TransactionID) (err error) {
	defer mon.Task()(&ctx)(&err)

	query := db.db.Rebind(` 
		WITH intent AS (
			SELECT tx_id, state FROM stripecoinpayments_apply_balance_intents WHERE tx_id = ? 
		), updated AS (
			UPDATE stripecoinpayments_apply_balance_intents AS ints
				SET 
					state = ? 
				FROM intent
				WHERE intent.tx_id = ints.tx_id  AND ints.state = ?
			RETURNING 1
		)
		SELECT EXISTS(SELECT 1 FROM intent) AS intent_exists, EXISTS(SELECT 1 FROM updated) AS intent_consumed;
	`)

	row := db.db.QueryRowContext(ctx, query, id, applyBalanceIntentStateConsumed, applyBalanceIntentStateUnapplied)

	var exists, consumed bool
	if err = row.Scan(&exists, &consumed); err != nil {
		return err
	}

	if !exists {
		return errs.New("can not consume transaction without apply balance intent")
	}
	if !consumed {
		return stripecoinpayments.ErrTransactionConsumed
	}

	return err
}

// LockRate locks conversion rate for transaction.
func (db *coinPaymentsTransactions) LockRate(ctx context.Context, id coinpayments.TransactionID, rate decimal.Decimal) (err error) {
	defer mon.Task()(&ctx)(&err)

	buff, err := rate.BigFloat().GobEncode()
	if err != nil {
		return errs.Wrap(err)
	}

	_, err = db.db.Create_StripecoinpaymentsTxConversionRate(ctx,
		dbx.StripecoinpaymentsTxConversionRate_TxId(id.String()),
		dbx.StripecoinpaymentsTxConversionRate_Rate(buff))

	return err
}

// GetLockedRate returns locked conversion rate for transaction or error if non exists.
func (db *coinPaymentsTransactions) GetLockedRate(ctx context.Context, id coinpayments.TransactionID) (_ decimal.Decimal, err error) {
	defer mon.Task()(&ctx)(&err)

	dbxRate, err := db.db.Get_StripecoinpaymentsTxConversionRate_By_TxId(ctx,
		dbx.StripecoinpaymentsTxConversionRate_TxId(id.String()),
	)
	if err != nil {
		return decimal.Decimal{}, err
	}

	var rateF big.Float
	if err = rateF.GobDecode(dbxRate.Rate); err != nil {
		return decimal.Decimal{}, errs.Wrap(err)
	}
	rate, err := monetary.DecimalFromBigFloat(&rateF)
	if err != nil {
		return decimal.Decimal{}, errs.Wrap(err)
	}

	return rate, nil
}

// ListAccount returns all transaction for specific user.
func (db *coinPaymentsTransactions) ListAccount(ctx context.Context, userID uuid.UUID) (_ []stripecoinpayments.Transaction, err error) {
	defer mon.Task()(&ctx)(&err)

	dbxTXs, err := db.db.All_CoinpaymentsTransaction_By_UserId_OrderBy_Desc_CreatedAt(ctx,
		dbx.CoinpaymentsTransaction_UserId(userID[:]),
	)
	if err != nil {
		return nil, err
	}

	var txs []stripecoinpayments.Transaction
	for _, dbxTX := range dbxTXs {
		tx, err := fromDBXCoinpaymentsTransaction(dbxTX)
		if err != nil {
			return nil, errs.Wrap(err)
		}

		txs = append(txs, *tx)
	}

	return txs, nil
}

// ListPending returns paginated list of pending transactions.
func (db *coinPaymentsTransactions) ListPending(ctx context.Context, offset int64, limit int, before time.Time) (_ stripecoinpayments.TransactionsPage, err error) {
	defer mon.Task()(&ctx)(&err)

	query := db.db.Rebind(`SELECT 
				id,
				user_id,
				address,
				amount,
				received,
				status,
				key,
				created_at
			FROM coinpayments_transactions 
			WHERE status IN (?,?)
			AND created_at <= ?
			ORDER by created_at DESC
			LIMIT ? OFFSET ?`)

	rows, err := db.db.QueryContext(ctx, query, coinpayments.StatusPending, coinpayments.StatusReceived, before, limit+1, offset)
	if err != nil {
		return stripecoinpayments.TransactionsPage{}, err
	}

	defer func() {
		err = errs.Combine(err, rows.Close())
	}()

	var page stripecoinpayments.TransactionsPage

	for rows.Next() {
		var id, address string
		var userID uuid.UUID
		var amountB, receivedB []byte
		var status int
		var key string
		var createdAt time.Time

		err := rows.Scan(&id, &userID, &address, &amountB, &receivedB, &status, &key, &createdAt)
		if err != nil {
			return stripecoinpayments.TransactionsPage{}, err
		}

		// TODO: the currency here should be passed in to this function or stored
		//  in the database.
		currency := monetary.StorjToken

		amount, err := monetaryAmountFromGobEncodedBigFloat(amountB, currency)
		if err != nil {
			return stripecoinpayments.TransactionsPage{}, err
		}
		received, err := monetaryAmountFromGobEncodedBigFloat(receivedB, currency)
		if err != nil {
			return stripecoinpayments.TransactionsPage{}, err
		}

		page.Transactions = append(page.Transactions,
			stripecoinpayments.Transaction{
				ID:        coinpayments.TransactionID(id),
				AccountID: userID,
				Address:   address,
				Amount:    amount,
				Received:  received,
				Status:    coinpayments.Status(status),
				Key:       key,
				CreatedAt: createdAt,
			},
		)
	}

	if err = rows.Err(); err != nil {
		return stripecoinpayments.TransactionsPage{}, err
	}

	if len(page.Transactions) == limit+1 {
		page.Next = true
		page.NextOffset = offset + int64(limit)
		page.Transactions = page.Transactions[:len(page.Transactions)-1]
	}

	return page, nil
}

// ListUnapplied returns TransactionsPage with a pending or completed status, that should be applied to account balance.
func (db *coinPaymentsTransactions) ListUnapplied(ctx context.Context, offset int64, limit int, before time.Time) (_ stripecoinpayments.TransactionsPage, err error) {
	defer mon.Task()(&ctx)(&err)

	query := db.db.Rebind(`SELECT 
				txs.id,
				txs.user_id,
				txs.address,
				txs.amount,
				txs.received,
				txs.status,
				txs.key,
				txs.created_at
			FROM coinpayments_transactions as txs 
			INNER JOIN stripecoinpayments_apply_balance_intents as ints
			ON txs.id = ints.tx_id
			WHERE txs.status >= ?
			AND txs.created_at <= ?
			AND ints.state = ?
			ORDER by txs.created_at DESC
			LIMIT ? OFFSET ?`)

	rows, err := db.db.QueryContext(ctx, query, coinpayments.StatusReceived, before, applyBalanceIntentStateUnapplied, limit+1, offset)
	if err != nil {
		return stripecoinpayments.TransactionsPage{}, err
	}
	defer func() { err = errs.Combine(err, rows.Close()) }()

	var page stripecoinpayments.TransactionsPage

	for rows.Next() {
		var id, address string
		var userID uuid.UUID
		var amountB, receivedB []byte
		var status int
		var key string
		var createdAt time.Time

		err := rows.Scan(&id, &userID, &address, &amountB, &receivedB, &status, &key, &createdAt)
		if err != nil {
			return stripecoinpayments.TransactionsPage{}, err
		}

		// TODO: the currency here should be passed in to this function or stored
		//  in the database.
		currency := monetary.StorjToken

		amount, err := monetaryAmountFromGobEncodedBigFloat(amountB, currency)
		if err != nil {
			return stripecoinpayments.TransactionsPage{}, errs.Wrap(err)
		}
		received, err := monetaryAmountFromGobEncodedBigFloat(receivedB, currency)
		if err != nil {
			return stripecoinpayments.TransactionsPage{}, errs.Wrap(err)
		}

		page.Transactions = append(page.Transactions,
			stripecoinpayments.Transaction{
				ID:        coinpayments.TransactionID(id),
				AccountID: userID,
				Address:   address,
				Amount:    amount,
				Received:  received,
				Status:    coinpayments.Status(status),
				Key:       key,
				CreatedAt: createdAt,
			},
		)
	}

	if err = rows.Err(); err != nil {
		return stripecoinpayments.TransactionsPage{}, err
	}

	if len(page.Transactions) == limit+1 {
		page.Next = true
		page.NextOffset = offset + int64(limit)
		page.Transactions = page.Transactions[:len(page.Transactions)-1]
	}

	return page, nil
}

// fromDBXCoinpaymentsTransaction converts *dbx.CoinpaymentsTransaction to *stripecoinpayments.Transaction.
func fromDBXCoinpaymentsTransaction(dbxCPTX *dbx.CoinpaymentsTransaction) (*stripecoinpayments.Transaction, error) {
	userID, err := uuid.FromBytes(dbxCPTX.UserId)
	if err != nil {
		return nil, errs.Wrap(err)
	}

	// TODO: the currency here should be passed in to this function or stored
	//  in the database.
	currency := monetary.StorjToken

	amount, err := monetaryAmountFromGobEncodedBigFloat(dbxCPTX.Amount, currency)
	if err != nil {
		return nil, errs.Wrap(err)
	}
	received, err := monetaryAmountFromGobEncodedBigFloat(dbxCPTX.Received, currency)
	if err != nil {
		return nil, errs.Wrap(err)
	}

	return &stripecoinpayments.Transaction{
		ID:        coinpayments.TransactionID(dbxCPTX.Id),
		AccountID: userID,
		Address:   dbxCPTX.Address,
		Amount:    amount,
		Received:  received,
		Status:    coinpayments.Status(dbxCPTX.Status),
		Key:       dbxCPTX.Key,
		Timeout:   time.Second * time.Duration(dbxCPTX.Timeout),
		CreatedAt: dbxCPTX.CreatedAt,
	}, nil
}

func monetaryAmountFromGobEncodedBigFloat(encoded []byte, currency *monetary.Currency) (_ monetary.Amount, err error) {
	var bf big.Float
	if err := bf.GobDecode(encoded); err != nil {
		return monetary.Amount{}, Error.Wrap(err)
	}
	return monetary.AmountFromBigFloat(&bf, currency)
}
