namespace go user

struct GetUserRequest {
    1: required i64 user_id
}

struct GetUserResponse {
    1: required i64 user_id
    2: required string name
    3: required i64 balance
}

service UserService {
    GetUserResponse GetUser(1: GetUserRequest req)
}
