package main

import (
	"fmt"
	"strings"
)

// ========================================
// 工厂方法模式 Factory Method Pattern
//
// 场景：消息通知系统
// 需要支持多种通知方式：Email、SMS、Slack
// 未来可能还要加 WeChat、钉钉...
//
// 核心思想：
//   - 定义一个创建对象的接口（工厂接口）
//   - 让具体工厂决定创建哪种产品
//   - 新增类型时只加代码，不改旧代码（开闭原则）
// ========================================

// ----------------------------------------
// 第一步：定义产品接口
// ----------------------------------------

type Notifier interface {
	Send(to, message string) error
	Name() string
}

// ----------------------------------------
// 第二步：实现具体产品
// ----------------------------------------

// --- Email 通知 ---
type EmailNotifier struct {
	smtpHost string
}

func (e *EmailNotifier) Send(to, message string) error {
	fmt.Printf("  📧 [Email] smtp://%s → %s\n     内容: %s\n", e.smtpHost, to, message)
	return nil
}
func (e *EmailNotifier) Name() string { return "Email" }

// --- SMS 通知 ---
type SMSNotifier struct {
	apiKey string
}

func (s *SMSNotifier) Send(to, message string) error {
	fmt.Printf("  📱 [SMS] apiKey:%s → %s\n     内容: %s\n", s.apiKey[:6]+"***", to, message)
	return nil
}
func (s *SMSNotifier) Name() string { return "SMS" }

// --- Slack 通知 ---
type SlackNotifier struct {
	webhookURL string
}

func (s *SlackNotifier) Send(to, message string) error {
	fmt.Printf("  💬 [Slack] webhook → #%s\n     内容: %s\n", to, message)
	return nil
}
func (s *SlackNotifier) Name() string { return "Slack" }

// ----------------------------------------
// 第三步：定义工厂接口
// ----------------------------------------

type NotifierFactory interface {
	Create() Notifier
}

// ----------------------------------------
// 第四步：实现具体工厂
// ----------------------------------------

type EmailFactory struct{ SmtpHost string }

func (f *EmailFactory) Create() Notifier {
	return &EmailNotifier{smtpHost: f.SmtpHost}
}

type SMSFactory struct{ APIKey string }

func (f *SMSFactory) Create() Notifier {
	return &SMSNotifier{apiKey: f.APIKey}
}

type SlackFactory struct{ WebhookURL string }

func (f *SlackFactory) Create() Notifier {
	return &SlackNotifier{webhookURL: f.WebhookURL}
}

// ----------------------------------------
// 第五步：简单工厂函数（Go 更常用的写法）
// 通过字符串/枚举直接拿到 Notifier
// ----------------------------------------

func NewNotifier(notifierType string) (Notifier, error) {
	switch strings.ToLower(notifierType) {
	case "email":
		return &EmailNotifier{smtpHost: "smtp.gmail.com"}, nil
	case "sms":
		return &SMSNotifier{apiKey: "sk-abc123xyz"}, nil
	case "slack":
		return &SlackNotifier{webhookURL: "https://hooks.slack.com/xxx"}, nil
	default:
		return nil, fmt.Errorf("未知的通知类型: %s", notifierType)
	}
}

// ----------------------------------------
// 业务代码：只依赖接口，不关心具体实现
// ----------------------------------------

type AlertService struct {
	notifier Notifier
}

func NewAlertService(factory NotifierFactory) *AlertService {
	return &AlertService{notifier: factory.Create()}
}

func (a *AlertService) SendAlert(to, msg string) {
	fmt.Printf("\n[AlertService] 使用 %s 发送告警\n", a.notifier.Name())
	if err := a.notifier.Send(to, msg); err != nil {
		fmt.Printf("  发送失败: %v\n", err)
	}
}

// ========================================
// main
// ========================================

func main() {
	fmt.Println("=== 工厂方法模式：通过工厂接口创建 ===")

	// 用不同工厂创建 AlertService，业务代码完全不变
	factories := []NotifierFactory{
		&EmailFactory{SmtpHost: "smtp.gmail.com"},
		&SMSFactory{APIKey: "sk-abc123xyz"},
		&SlackFactory{WebhookURL: "https://hooks.slack.com/xxx"},
	}

	for _, factory := range factories {
		svc := NewAlertService(factory)
		svc.SendAlert("ops-team", "服务器 CPU 超过 90%！")
	}

	fmt.Println("\n=== 简单工厂函数：Go 项目更常见的写法 ===")

	// 从配置/环境变量读取通知类型，动态创建
	types := []string{"email", "sms", "slack", "wechat"} // wechat 故意触发错误

	for _, t := range types {
		notifier, err := NewNotifier(t)
		if err != nil {
			fmt.Printf("\n  ❌ 创建失败: %v\n", err)
			continue
		}
		fmt.Println()
		notifier.Send("admin@example.com", "这是一条测试消息")
	}

	fmt.Println("\n=== 关键验证：业务代码对具体类型无感知 ===")
	// sendNotification 只认识 Notifier 接口
	// 传什么进来都能工作，这就是工厂方法的价值
	sendNotification := func(n Notifier, to, msg string) {
		fmt.Printf("发送中 [%s]... ", n.Name())
		n.Send(to, msg)
	}

	email, _ := NewNotifier("email")
	sms, _ := NewNotifier("sms")
	sendNotification(email, "user@example.com", "验证码：8848")
	sendNotification(sms, "+8613800138000", "验证码：8848")
}
