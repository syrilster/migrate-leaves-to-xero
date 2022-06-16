package xero

type EmpResponse struct {
	Status    string     `json:"Status"`
	Employees []Employee `json:"Employees"`
}

type ConnectionResp struct {
	Connections []Connection
}

type Employee struct {
	EmployeeID        string         `json:"EmployeeID"`
	FirstName         string         `json:"FirstName"`
	LastName          string         `json:"LastName"`
	Status            string         `json:"Status"`
	PayrollCalendarID string         `json:"PayrollCalendarID"`
	LeaveBalance      []LeaveBalance `json:"LeaveBalances"`
}

type Connection struct {
	TenantID   string `json:"tenantId"`
	TenantType string `json:"tenantType"`
	OrgName    string `json:"tenantName"`
}

type LeaveBalanceResponse struct {
	Employees []Employee `json:"Employees"`
}

type LeaveBalance struct {
	LeaveType     string  `json:"LeaveName"`
	LeaveTypeID   string  `json:"LeaveTypeID"`
	NumberOfUnits float64 `json:"NumberOfUnits"`
	TypeOfUnits   string  `json:"TypeOfUnits"`
}

type LeaveApplicationRequest struct {
	EmployeeID   string        `json:"EmployeeID"`
	LeaveTypeID  string        `json:"LeaveTypeID"`
	StartDate    string        `json:"StartDate"`
	EndDate      string        `json:"EndDate"`
	Title        string        `json:"Title"`
	LeavePeriods []LeavePeriod `json:"LeavePeriods"`
}

type LeavePeriod struct {
	PayPeriodEndDate string  `json:"PayPeriodEndDate"`
	NumberOfUnits    float64 `json:"NumberOfUnits"`
}

type PayrollCalendarResponse struct {
	PayrollCalendars []PayrollCalendar `json:"PayrollCalendars"`
}

type PayrollCalendar struct {
	PayrollCalendarID string `json:"PayrollCalendarID"`
	CalendarType      string `json:"CalendarType"`
	PaymentDate       string `json:"PaymentDate"`
}
