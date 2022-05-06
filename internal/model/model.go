package model

import "time"

type XeroResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type XeroConnections []Connection
type XeroEmployees []Employee

type Connection struct {
	TenantID   string `json:"tenantId"`
	TenantType string `json:"tenantType"`
	OrgName    string `json:"tenantName"`
}

type Employee struct {
	EmployeeID string `json:"EmployeeID"`
	FirstName  string `json:"FirstName"`
	LastName   string `json:"LastName"`
	Status     string `json:"Status"`
}

type KrowLeaveRequest struct {
	LeaveDate      time.Time
	LeaveDateEpoch int64
	Hours          float64
	LeaveType      string
	OrgName        string
	EmpName        string
	Description    string
}
