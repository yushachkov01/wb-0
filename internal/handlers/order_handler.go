package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-redis/redis/v8"
	_ "github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"golang_project/internal/data"
)

func WriteOrderToRedis(orderID string, order data.Order) (bool, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "1234", // Пароль, если есть
		DB:       0,      // Номер базы данных Redis
	})
	defer client.Close()

	exists := client.Exists(context.Background(), fmt.Sprintf("order:%s", orderID)).Val()
	if exists == 1 {
		return true, nil
	}

	orderJSON, err := json.Marshal(order)
	if err != nil {
		return false, fmt.Errorf("ошибка преобразования в JSON: %w", err)
	}

	err = client.Set(context.Background(), fmt.Sprintf("order:%s", orderID), orderJSON, 0).Err()
	if err != nil {
		return false, fmt.Errorf("ошибка записи в Redis: %w", err)
	}

	return false, nil
}

func GetOrderDetailsByIDHandler(w http.ResponseWriter, r *http.Request) {
	orderID := data.ExtractOrderID(r.URL.Path)
	if orderID == "" {
		http.Error(w, "Неверный путь", http.StatusBadRequest)
		return
	}

	//log.Printf("Извлеченный orderID: %s", orderID)

	connStr := "user=yushachkov password=1234 dbname=levelzerodb sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	query := `SELECT order_uid, track_number, entry, 
		delivery_name, delivery_phone, delivery_zip, delivery_city, delivery_address, delivery_region, delivery_email,
		payment_transaction, payment_request_id, payment_currency, payment_provider, payment_amount, payment_payment_dt, payment_bank, payment_delivery_cost, payment_goods_total, payment_custom_fee,
		locale, internal_signature, customer_id, delivery_service, shardkey, sm_id, date_created, oof_shard
    FROM orders WHERE order_uid = $1`

	rows, err := db.Query(query, orderID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var orders []data.Order
	for rows.Next() {
		var order data.Order
		var items []data.Item

		err := rows.Scan(
			&order.OrderUID, &order.TrackNumber, &order.Entry,
			&order.Delivery.Name, &order.Delivery.Phone, &order.Delivery.Zip, &order.Delivery.City, &order.Delivery.Address, &order.Delivery.Region, &order.Delivery.Email,
			&order.Payment.Transaction, &order.Payment.RequestID, &order.Payment.Currency, &order.Payment.Provider, &order.Payment.Amount, &order.Payment.PaymentDt, &order.Payment.Bank, &order.Payment.DeliveryCost, &order.Payment.GoodsTotal, &order.Payment.CustomFee,
			&order.Locale, &order.InternalSig, &order.CustomerID, &order.DeliveryServ, &order.Shardkey, &order.SmID, &order.DateCreated, &order.OofShard,
		)
		if err != nil {
			panic(err)
		}

		itemsQuery := `SELECT items_chrt_id, items_track_number, items_price, items_rid, items_name, items_sale, items_size, items_total_price, items_nm_id, items_brand, items_status FROM orders WHERE order_uid = $1`
		rowsItems, err := db.Query(itemsQuery, order.OrderUID)
		if err != nil {
			panic(err)
		}
		defer rowsItems.Close()

		for rowsItems.Next() {
			var item data.Item
			err := rowsItems.Scan(
				&item.ChrtID, &item.TrackNumber, &item.Price, &item.RID, &item.Name, &item.Sale, &item.Size, &item.TotalPrice, &item.NmID, &item.Brand, &item.Status,
			)
			if err != nil {
				panic(err)
			}
			items = append(items, item)
		}
		order.Items = items

		exists, err := WriteOrderToRedis(order.OrderUID, order)
		if err != nil {
			log.Printf("Ошибка при записи заказа в Redis: %v", err)
		}

		if exists {
			log.Printf("Данные для заказа %s уже существуют в Redis", order.OrderUID)
		}

		orders = append(orders, order)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(orders)
}
