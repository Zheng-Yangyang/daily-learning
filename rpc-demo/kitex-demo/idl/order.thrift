namespace go order

struct CreateOrderRequest {
    1: required i64 user_id
    2: required double amount
    3: required string item_name
}

struct CreateOrderResponse {
    1: required string order_no
    2: required string status
}

service OrderService {
    CreateOrderResponse CreateOrder(1: CreateOrderRequest req)
}
