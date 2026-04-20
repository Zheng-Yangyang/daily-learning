package api

import (
	"net/http"
	"payment-system/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	paymentSvc *service.PaymentService
}

func NewHandler(paymentSvc *service.PaymentService) *Handler {
	return &Handler{paymentSvc: paymentSvc}
}

// CreateOrder POST /orders
func (h *Handler) CreateOrder(c *gin.Context) {
	var req service.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	order, err := h.paymentSvc.CreateOrder(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "下单成功",
		"data":    order,
	})
}

// Pay POST /pay
func (h *Handler) Pay(c *gin.Context) {
	var req service.PayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	payment, err := h.paymentSvc.Pay(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "支付成功",
		"data":    payment,
	})
}

// GetUser GET /users/:id
func (h *Handler) GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的用户ID"})
		return
	}

	// 直接通过 service 层的 db 查询，简化演示
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    gin.H{"user_id": id},
	})
}
