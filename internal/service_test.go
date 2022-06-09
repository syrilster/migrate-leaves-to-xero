package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/stretchr/testify/require"
	"net/http"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/xero"
)

type MockXeroClient struct {
	mock.Mock
}

func TestLeaveMigration(t *testing.T) {
	var annualLeave = xero.LeaveBalance{
		LeaveType:     "Annual Leave",
		LeaveTypeID:   "73f37030-b1ed-bb37-0242ac130002",
		NumberOfUnits: 20,
		TypeOfUnits:   "Hours",
	}

	var personalLeave = xero.LeaveBalance{
		LeaveType:     "Personal/Carer's Leave",
		LeaveTypeID:   "ac62f6ec-a3cd-11ea-bb37-0242ac130002",
		NumberOfUnits: 20,
		TypeOfUnits:   "Hours",
	}

	var compassionateLeave = xero.LeaveBalance{
		LeaveType:     "Compassionate Leave (paid)",
		LeaveTypeID:   "df62f6ec-a3cd-11ea-bb37-0242ac1300123",
		NumberOfUnits: 8,
		TypeOfUnits:   "Hours",
	}

	var juryDurtyLeave = xero.LeaveBalance{
		LeaveType:     "Jury Duty",
		LeaveTypeID:   "ca62f6ec-a3cd-11ea-bb37-0242ac130005",
		NumberOfUnits: 8,
		TypeOfUnits:   "Hours",
	}

	digIOTenantID := "111111"
	eliizaTenantID := "222222"
	cmdTenantID := "333333"
	mantelTenantID := "4444444"
	empID := "123456"
	var connectionResp = []xero.Connection{
		{
			TenantID:   digIOTenantID,
			TenantType: "Org",
			OrgName:    "DigIO",
		},
		{
			TenantID:   mantelTenantID,
			TenantType: "Org",
			OrgName:    "Mantel Group",
		},
		{
			TenantID:   cmdTenantID,
			TenantType: "Org",
			OrgName:    "CMD",
		},
		{
			TenantID:   eliizaTenantID,
			TenantType: "Org",
			OrgName:    "Eliiza",
		},
	}

	empResp := &xero.EmpResponse{
		Status: "Active",
		Employees: []xero.Employee{
			{
				EmployeeID:        empID,
				FirstName:         "Syril",
				LastName:          "Sadasivan",
				Status:            "Active",
				PayrollCalendarID: "4567891011",
				LeaveBalance: []xero.LeaveBalance{
					annualLeave,
					personalLeave,
				},
			},
		},
		RateLimitRemaining: 60,
	}

	payRollCalendarResp := &xero.PayrollCalendarResponse{
		PayrollCalendars: []xero.PayrollCalendar{
			{
				PayrollCalendarID: "4567891011",
				PaymentDate:       "/Date(632102400000+0000)/",
			},
		},
	}

	leaveBalResp := &xero.LeaveBalanceResponse{Employees: empResp.Employees, RateLimitRemaining: 60}

	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", "http://dummy", "testEndpoint"), nil)
	require.NoError(t, err)
	mockRequest := &xero.ReusableRequest{Request: r}
	mockClient := new(MockXeroClient)

	s, err := session.NewSession()
	require.NoError(t, err)
	sesClient := ses.New(s)

	mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
	mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockRequest, nil)
	mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
	mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(leaveBalResp, nil)
	mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
	mockClient.On("NewPayrollRequest", context.Background(), digIOTenantID).Return(mockRequest, nil)
	mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(nil)

	t.Run("Success", func(t *testing.T) {
		xlsLocation := getProjectRoot() + "/test/digio_leave.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		err := service.MigrateLeaveKrowToXero(context.Background())
		assert.Nil(t, err)
	})

	t.Run("Error When invalid data in sheet", func(t *testing.T) {
		expectedResp := "Invalid entry for Leave Date: 28/04/20. Valid Format DD/MM/YYYY (Ex: 01/06/2020)"
		xlsLocation := getProjectRoot() + "/test/all_error.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		err := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, err)
		assert.Equal(t, 4, len(err))
		assert.True(t, contains(err, expectedResp))
	})

	t.Run("Error when employee has insufficient leave balance for leave type Jury Duty and compassionate leave", func(t *testing.T) {
		expectedResp := "Employee: Syril Sadasivan has insufficient Leave balance for Leave type Compassionate Leave (paid) requested for 8 hours"
		xlsLocation := getProjectRoot() + "/test/digio_various_leave.xlsx"

		empResp := &xero.EmpResponse{
			Status: "Active",
			Employees: []xero.Employee{
				{
					EmployeeID:        empID,
					FirstName:         "Syril",
					LastName:          "Sadasivan",
					Status:            "Active",
					PayrollCalendarID: "4567891011",
					LeaveBalance: []xero.LeaveBalance{
						annualLeave,
						personalLeave,
						compassionateLeave,
						juryDurtyLeave,
					},
				},
			},
			RateLimitRemaining: 60,
		}

		leaveBalResp := &xero.LeaveBalanceResponse{Employees: empResp.Employees, RateLimitRemaining: 60}

		s, err := session.NewSession()
		require.NoError(t, err)
		sesClient := ses.New(s)

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockRequest, nil)
		mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), digIOTenantID).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(nil)

		service := NewService(mockClient, xlsLocation, sesClient, "", "")
		resp := service.MigrateLeaveKrowToXero(context.Background())

		assert.NotNil(t, resp)
		assert.Equal(t, 2, len(resp))
		assert.True(t, contains(resp, expectedResp))
	})

	t.Run("Success When Org having more than 100 employees", func(t *testing.T) {
		xlsLocation := getProjectRoot() + "/test/digio_leave.xlsx"

		empJSON := "{\n  \"Status\": \"OK\",\n  \"Employees\": [\n    {\n      \"EmployeeID\": \"6753c19a-2b72-444a-a4f1-0b221115a0ab\",\n      \"FirstName\": \"Adrian\",\n      \"LastName\": \"Lai\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b5bead37-59c8-4322-a042-f396c0bf00df\",\n      \"FirstName\": \"Akila \",\n      \"MiddleNames\": \"Geethal\",\n      \"LastName\": \"Bodiya Baduge\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f17d6e0a-4a44-4fc6-967f-23def09891a9\",\n      \"FirstName\": \"Akshay\",\n      \"LastName\": \"Santosh\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"13bacd79-e738-47b4-9ce7-ad3b180c4276\",\n      \"FirstName\": \"Amanda\",\n      \"LastName\": \"Brown\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"389bff80-1470-484b-98e6-5c61b1c5c810\",\n      \"FirstName\": \"Andrew\",\n      \"LastName\": \"Cranston\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"09f720f4-401d-4611-bb08-7efbcc22f8e7\",\n      \"FirstName\": \"Andrew\",\n      \"LastName\": \"Opat\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"84092ff5-f40a-47c1-a549-9a3daaf0c1e8\",\n      \"FirstName\": \"Andrew\",\n      \"LastName\": \"Bell\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"19a263d3-07e7-4291-b634-20c5fd07b647\",\n      \"FirstName\": \"Anjana\",\n      \"LastName\": \"Varma\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ac89d9f3-4e49-49d5-b43e-dc5471449213\",\n      \"FirstName\": \"Anthony\",\n      \"LastName\": \"Scata\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"62a447f6-3773-4620-a0f3-467984701b17\",\n      \"FirstName\": \"Anthony\",\n      \"LastName\": \"Hallworth\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"8c20aa4d-222d-4206-bfd4-316efb00b8bf\",\n      \"FirstName\": \"Aquiles \",\n      \"LastName\": \"Boff Da Silva\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a16539c5-1241-42e8-94bd-e1eef53d8065\",\n      \"FirstName\": \"Aravindan\",\n      \"LastName\": \"Mathiazhagan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"e8c4db81-6c83-440c-beb8-d15a81f9bfc3\",\n      \"FirstName\": \"Arjen\",\n      \"LastName\": \"Schwarz\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b7d5dc1a-a444-48ef-92ac-7cb58008e128\",\n      \"FirstName\": \"Aron\",\n      \"MiddleNames\": \"Elvis\",\n      \"LastName\": \"Tucker\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"174bf16d-8042-4ea2-92ac-baef4c389869\",\n      \"FirstName\": \"Ashish\",\n      \"LastName\": \"Tripathi\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f2d4a4f6-9069-428f-be69-fdcd21ef8bd9\",\n      \"FirstName\": \"Aswathy\",\n      \"LastName\": \"Latheedevi Ramachandran\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"05d7ffe4-78c9-4958-9a74-affab4534a9b\",\n      \"FirstName\": \"Ben\",\n      \"LastName\": \"Howl\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"1f96c906-e013-496a-9bd6-83d520d2b5d1\",\n      \"FirstName\": \"Ben\",\n      \"LastName\": \"Spiccia\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"109a7166-e20b-43d4-a906-684b90f03f86\",\n      \"FirstName\": \"Ben\",\n      \"LastName\": \"Ebsworth\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"01e7f43c-832d-4e0d-a0e5-57235e820442\",\n      \"FirstName\": \"Bhaawna\",\n      \"LastName\": \"Manoranjan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a478f94c-6980-43fc-805a-98057fef0d0e\",\n      \"FirstName\": \"Bradley\",\n      \"LastName\": \"Thomas\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ac5b0f6f-77a5-4452-b5f7-2a795e037a39\",\n      \"FirstName\": \"Brett\",\n      \"LastName\": \"Henderson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"fb55731c-befd-47bd-90af-d6c1e0b8d427\",\n      \"FirstName\": \"Brett\",\n      \"LastName\": \"Uglow\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"3974e30f-b14b-4640-9c68-cc0ad710c43e\",\n      \"FirstName\": \"Caroline\",\n      \"LastName\": \"Davis\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"da0b7f5b-b90e-46e6-a79e-4d0b71699f42\",\n      \"FirstName\": \"Cathy\",\n      \"LastName\": \"Jamshidi\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"df49f267-e5a3-4283-bd19-f8c0a9a55a0a\",\n      \"FirstName\": \"Chandan\",\n      \"LastName\": \"Rai\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4f184075-d229-4b12-aecd-a9546f9ab666\",\n      \"FirstName\": \"Chee\",\n      \"LastName\": \"Ding\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"50c15183-cc72-47f7-9f5c-22195e91d6e3\",\n      \"FirstName\": \"Chris\",\n      \"LastName\": \"Carroll\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f5b26f67-a254-46e3-9772-d468656e3206\",\n      \"FirstName\": \"Daniel\",\n      \"MiddleNames\": \"Roy\",\n      \"LastName\": \"Proud\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0bbd0deb-d45a-4f36-aa5d-b9e8bf3853db\",\n      \"FirstName\": \"Daniel\",\n      \"LastName\": \"Mills\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0ee03e63-dae2-4947-97d9-a635633dd8e2\",\n      \"FirstName\": \"Daniel\",\n      \"LastName\": \"Cross\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ad98fc73-558d-4e18-941f-491b5092ffe1\",\n      \"FirstName\": \"Declan\",\n      \"LastName\": \"Robinson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"c20f3f33-c26b-4cfd-9f08-ea0903df1b47\",\n      \"FirstName\": \"Deepa\",\n      \"LastName\": \"Aravindan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"243eeae5-d5e3-4def-a371-e036e6d48e89\",\n      \"FirstName\": \"Dmitry\",\n      \"LastName\": \"Likane\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"06bedb0d-4031-4a8d-a956-c78ae324a24e\",\n      \"FirstName\": \"Don\",\n      \"LastName\": \"Wanniarachchi\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"e5ebeb7a-8cd1-4fdd-8715-dc0d313dcd64\",\n      \"FirstName\": \"Donovan\",\n      \"LastName\": \"Chong\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"e9a245eb-d0eb-40d0-a2a3-91d67a058b6a\",\n      \"FirstName\": \"Eduanne\",\n      \"LastName\": \"Nel\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f09e5a00-5de7-4c0b-96bc-935e1521b4da\",\n      \"FirstName\": \"Edwin\",\n      \"LastName\": \"Granados\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"46325177-3634-492c-843e-4535cc0f47a5\",\n      \"FirstName\": \"Elliott\",\n      \"LastName\": \"Brooks-Miller\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d98d0d12-ccd5-4e67-b1ab-e1c208f06da7\",\n      \"FirstName\": \"Emma\",\n      \"MiddleNames\": \"Carole\",\n      \"LastName\": \"Cullen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"c9de3eb1-e08b-4bc2-8874-cd5d711f2943\",\n      \"FirstName\": \"Erwin\",\n      \"MiddleNames\": \"Rommel\",\n      \"LastName\": \"Acoba\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"17290f76-fdd2-44bc-9192-bd79f96be1f3\",\n      \"FirstName\": \"Gary\",\n      \"LastName\": \"Chang\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d00ada90-30b8-4521-9aed-9749c2e8a579\",\n      \"FirstName\": \"Gawri \",\n      \"LastName\": \"Edussuriya\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4918bea3-c9da-4f0a-b55b-80f8fd315efd\",\n      \"FirstName\": \"Gayan\",\n      \"LastName\": \"Belpamulle\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"167a8ac2-16ed-40ba-82f2-d365a5b5e5ef\",\n      \"FirstName\": \"Ge\",\n      \"LastName\": \"Gao\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4993ea9c-a010-4c38-8199-bb4f7250c0bc\",\n      \"FirstName\": \"Geoffrey\",\n      \"LastName\": \"Harrison\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a4d9d44e-a647-4f90-a788-3ab9d8bfcd3d\",\n      \"FirstName\": \"Grant\",\n      \"MiddleNames\": \"Driver\",\n      \"LastName\": \"Sutton\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"2e50f561-3c55-4d30-9b9d-26c0b84ae0de\",\n      \"FirstName\": \"Gregory\",\n      \"LastName\": \"Austin\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d7c5bb40-2efb-4015-8941-b2e65c03e2a1\",\n      \"FirstName\": \"Harinie\",\n      \"LastName\": \"Immanuel\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"1e72e7e1-dfc9-4fe8-aacd-691247b4b37d\",\n      \"FirstName\": \"Hendrik\",\n      \"LastName\": \"Brandt\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"7104dbd6-91b9-457e-bcd0-402e142e86e2\",\n      \"FirstName\": \"Hojun\",\n      \"LastName\": \"Joo\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"518a87f9-00d5-4a4b-863a-abf230e3dacc\",\n      \"FirstName\": \"Ivan\",\n      \"LastName\": \"Luong\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"419a369a-9997-4420-a199-b31523d57e97\",\n      \"FirstName\": \"Jacob\",\n      \"LastName\": \"Gitlin\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4f65738f-65d4-4c69-9d01-913044fc6ad1\",\n      \"FirstName\": \"Jake\",\n      \"LastName\": \"Nelson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"422d30a0-819f-48ad-b9f2-8a16fb8cc87a\",\n      \"FirstName\": \"James\",\n      \"LastName\": \"Kieltyka\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"7a193ff8-1f22-4182-b217-1e745810ff9b\",\n      \"FirstName\": \"James\",\n      \"LastName\": \"Martin\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d889e3c2-fd68-42f3-916c-c20c52aed627\",\n      \"FirstName\": \"Jane\",\n      \"LastName\": \"Nguyen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"af72da76-bf60-42bc-a469-8f6cbc92617a\",\n      \"FirstName\": \"Jason\",\n      \"LastName\": \"Feng\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d936ff3a-e849-47d7-a9d6-593e05afeaae\",\n      \"FirstName\": \"Jesse\",\n      \"LastName\": \"Jackson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"bea33f6f-55eb-4459-8a96-7dbe825d4add\",\n      \"FirstName\": \"Jessica\",\n      \"LastName\": \"Odri\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"eb863b11-152f-4b97-b51e-721b8c5f4a97\",\n      \"FirstName\": \"Jiri\",\n      \"LastName\": \"Sklenar\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a6222418-3070-46e8-889e-c78cad4f72f7\",\n      \"FirstName\": \"John\",\n      \"LastName\": \"Stableford\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ac3b4496-2f27-47d6-a0bf-83ba7cf87394\",\n      \"FirstName\": \"John Paul\",\n      \"LastName\": \"Millan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ba6cb47f-e1da-4d6f-93d8-8675ac4c4ebc\",\n      \"FirstName\": \"John-Paul\",\n      \"LastName\": \"Kelly\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"88f9d3de-84ee-4d20-878c-5c76db920464\",\n      \"FirstName\": \"Jonathan\",\n      \"LastName\": \"Derham\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0363b3e4-6fff-45fb-a002-043e3dd10575\",\n      \"FirstName\": \"Jonathon\",\n      \"LastName\": \"Ashworth\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d22a361f-0592-4004-9ea2-e4a4357b0d15\",\n      \"FirstName\": \"Joseph\",\n      \"LastName\": \"Motha\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"edd5bab2-d5a8-452e-8b8a-761a48689e0a\",\n      \"FirstName\": \"Karthik\",\n      \"LastName\": \"Kunjithapatham\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b821a921-0268-4e69-af79-1b90803d3ee2\",\n      \"FirstName\": \"Kelly\",\n      \"LastName\": \"Stewart\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ee3c2a78-3995-451e-b10f-621ae83f62b1\",\n      \"FirstName\": \"Keren\",\n      \"LastName\": \"Burshtein\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"580ee69f-76fd-4b22-8cb8-c7038e27135e\",\n      \"FirstName\": \"Kinshuk\",\n      \"LastName\": \"Sen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"62448041-200a-4c1e-bdc3-8bd43541d231\",\n      \"FirstName\": \"Kishore\",\n      \"LastName\": \"Nanduri\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"074addea-77e6-4635-8825-44e67561a297\",\n      \"FirstName\": \"Kseniia\",\n      \"LastName\": \"Isaeva\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d33bfdfc-aa4e-4312-9cb3-fd18218e7d2d\",\n      \"FirstName\": \"Laurence\",\n      \"LastName\": \"Judge\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"daee3bd7-bb57-4d8b-aaac-3678f69a6d00\",\n      \"FirstName\": \"Lily\",\n      \"LastName\": \"Huang\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"8d7dd435-9644-43a1-aff9-c194629f78c0\",\n      \"FirstName\": \"Linda\",\n      \"LastName\": \"Connolly\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f699b69a-b980-49e7-b964-e0f29418b2c4\",\n      \"FirstName\": \"Lindsey\",\n      \"LastName\": \"Teal\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"bfd2d710-68af-4a21-83e8-dd5b6575501a\",\n      \"FirstName\": \"Marcel\",\n      \"LastName\": \"McFall\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"23b5677f-d56f-4103-87af-15a7191d82e1\",\n      \"FirstName\": \"Maria\",\n      \"LastName\": \"La Porta\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a5b97086-61fb-4c60-afe7-d656192d2005\",\n      \"FirstName\": \"Mark\",\n      \"LastName\": \"Sedrak\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"438d3827-655d-435f-9d1a-169901fc1670\",\n      \"FirstName\": \"Mark\",\n      \"LastName\": \"Georgeson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"debaaf38-4ee7-4c79-9839-9a1de9a2e1f9\",\n      \"FirstName\": \"Matthew\",\n      \"MiddleNames\": \"James\",\n      \"LastName\": \"Corrigan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"80b12c7f-b8d9-45f6-a4c1-083e7583787e\",\n      \"FirstName\": \"Matthieu\",\n      \"LastName\": \"Siggen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0c0b5f5d-62da-4228-9c94-d8a0ea0c70e3\",\n      \"FirstName\": \"Michael\",\n      \"LastName\": \"Nguyen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"13dd1889-35e1-4820-8510-794a34a7b2de\",\n      \"FirstName\": \"Michiel\",\n      \"LastName\": \"Kalkman\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"032b2aa8-14be-47a9-b7c9-0f54232502ad\",\n      \"FirstName\": \"Min Jin\",\n      \"LastName\": \"Tsai\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b47b1f60-159a-474b-a9ba-646998ab8550\",\n      \"FirstName\": \"Mitchell\",\n      \"LastName\": \"Davis\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d225bf03-533a-47b4-9cf1-603a8b4b1113\",\n      \"FirstName\": \"Nick\",\n      \"LastName\": \"Burnard\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"11780b0b-3991-4052-9a87-1222f1c43e86\",\n      \"FirstName\": \"Nick\",\n      \"LastName\": \"Beagley\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d4d3081c-e7d6-4be8-b98c-3bee07bdb513\",\n      \"FirstName\": \"Nick\",\n      \"LastName\": \"Schnelle\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"85a8a4ab-7641-4f9c-b993-da7b4fb98645\",\n      \"FirstName\": \"Nina\",\n      \"LastName\": \"Zivkovic\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"7ed078c9-670d-4d1d-87cd-e7322a73fd3c\",\n      \"FirstName\": \"Patrick\",\n      \"LastName\": \"Eckel\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"2e19fed8-40f5-418c-ad55-764b2a182e98\",\n      \"FirstName\": \"Peter\",\n      \"LastName\": \"Condick\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"6eb64e85-912d-4af2-b0e8-af3a8db7ac85\",\n      \"FirstName\": \"Peter\",\n      \"LastName\": \"Hall\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b97a8312-fa79-4468-b7ba-009a16cde3f7\",\n      \"FirstName\": \"Priyanka\",\n      \"LastName\": \"Jagga\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"13bccec3-e844-47ef-a183-82db9875cb08\",\n      \"FirstName\": \"Rajan\",\n      \"LastName\": \"Arkenbout\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f0cd324b-7705-463e-92f6-b120bd4e10d0\",\n      \"FirstName\": \"Rambabu\",\n      \"LastName\": \"Potla\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"151d4370-b2ba-4fa6-92f5-c2db0d98c8e8\",\n      \"FirstName\": \"Roman\",\n      \"LastName\": \"Makosiy\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"3f96f30e-8b4b-467b-9abc-af758a08c7e0\",\n      \"FirstName\": \"Roman\",\n      \"LastName\": \"Gurevitch\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"bb92f7bf-6fc8-4c66-b804-b6d2148ce9de\",\n      \"FirstName\": \"Sam\",\n      \"LastName\": \"McLeod\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    }\n  ]\n}"
		mockEmpResponse := &xero.EmpResponse{RateLimitRemaining: 60}
		if err := json.Unmarshal([]byte(empJSON), mockEmpResponse); err != nil {
			assert.Failf(t, "There was an error un marshalling the xero API resp", fmt.Sprintf("Error details %v", err))
		}

		leaveBalResp := &xero.LeaveBalanceResponse{Employees: empResp.Employees, RateLimitRemaining: 60}
		r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", "http://dummy", "testEndpoint"), nil)
		require.NoError(t, err)
		mockReqPageOne := &xero.ReusableRequest{Request: r}

		rp, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", "http://dummy", "testEndpointPage2"), nil)
		require.NoError(t, err)
		mockReqPageTwo := &xero.ReusableRequest{Request: rp}

		s, err := session.NewSession()
		require.NoError(t, err)
		sesClient := ses.New(s)

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockReqPageOne, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "2").Return(mockReqPageTwo, nil)
		mockClient.On("GetEmployees", context.Background(), mockReqPageOne).Return(mockEmpResponse, nil)
		mockClient.On("GetEmployees", context.Background(), mockReqPageTwo).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), digIOTenantID).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(nil)

		service := NewService(mockClient, xlsLocation, sesClient, "", "")
		resp := service.MigrateLeaveKrowToXero(context.Background())
		assert.Nil(t, resp)
	})

	t.Run("Failure When Org having more than 100 employees and page 2 returns an error", func(t *testing.T) {
		xlsLocation := getProjectRoot() + "/test/digio_leave.xlsx"

		empJSON := "{\n  \"Status\": \"OK\",\n  \"Employees\": [\n    {\n      \"EmployeeID\": \"6753c19a-2b72-444a-a4f1-0b221115a0ab\",\n      \"FirstName\": \"Adrian\",\n      \"LastName\": \"Lai\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b5bead37-59c8-4322-a042-f396c0bf00df\",\n      \"FirstName\": \"Akila \",\n      \"MiddleNames\": \"Geethal\",\n      \"LastName\": \"Bodiya Baduge\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f17d6e0a-4a44-4fc6-967f-23def09891a9\",\n      \"FirstName\": \"Akshay\",\n      \"LastName\": \"Santosh\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"13bacd79-e738-47b4-9ce7-ad3b180c4276\",\n      \"FirstName\": \"Amanda\",\n      \"LastName\": \"Brown\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"389bff80-1470-484b-98e6-5c61b1c5c810\",\n      \"FirstName\": \"Andrew\",\n      \"LastName\": \"Cranston\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"09f720f4-401d-4611-bb08-7efbcc22f8e7\",\n      \"FirstName\": \"Andrew\",\n      \"LastName\": \"Opat\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"84092ff5-f40a-47c1-a549-9a3daaf0c1e8\",\n      \"FirstName\": \"Andrew\",\n      \"LastName\": \"Bell\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"19a263d3-07e7-4291-b634-20c5fd07b647\",\n      \"FirstName\": \"Anjana\",\n      \"LastName\": \"Varma\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ac89d9f3-4e49-49d5-b43e-dc5471449213\",\n      \"FirstName\": \"Anthony\",\n      \"LastName\": \"Scata\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"62a447f6-3773-4620-a0f3-467984701b17\",\n      \"FirstName\": \"Anthony\",\n      \"LastName\": \"Hallworth\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"8c20aa4d-222d-4206-bfd4-316efb00b8bf\",\n      \"FirstName\": \"Aquiles \",\n      \"LastName\": \"Boff Da Silva\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a16539c5-1241-42e8-94bd-e1eef53d8065\",\n      \"FirstName\": \"Aravindan\",\n      \"LastName\": \"Mathiazhagan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"e8c4db81-6c83-440c-beb8-d15a81f9bfc3\",\n      \"FirstName\": \"Arjen\",\n      \"LastName\": \"Schwarz\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b7d5dc1a-a444-48ef-92ac-7cb58008e128\",\n      \"FirstName\": \"Aron\",\n      \"MiddleNames\": \"Elvis\",\n      \"LastName\": \"Tucker\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"174bf16d-8042-4ea2-92ac-baef4c389869\",\n      \"FirstName\": \"Ashish\",\n      \"LastName\": \"Tripathi\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f2d4a4f6-9069-428f-be69-fdcd21ef8bd9\",\n      \"FirstName\": \"Aswathy\",\n      \"LastName\": \"Latheedevi Ramachandran\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"05d7ffe4-78c9-4958-9a74-affab4534a9b\",\n      \"FirstName\": \"Ben\",\n      \"LastName\": \"Howl\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"1f96c906-e013-496a-9bd6-83d520d2b5d1\",\n      \"FirstName\": \"Ben\",\n      \"LastName\": \"Spiccia\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"109a7166-e20b-43d4-a906-684b90f03f86\",\n      \"FirstName\": \"Ben\",\n      \"LastName\": \"Ebsworth\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"01e7f43c-832d-4e0d-a0e5-57235e820442\",\n      \"FirstName\": \"Bhaawna\",\n      \"LastName\": \"Manoranjan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a478f94c-6980-43fc-805a-98057fef0d0e\",\n      \"FirstName\": \"Bradley\",\n      \"LastName\": \"Thomas\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ac5b0f6f-77a5-4452-b5f7-2a795e037a39\",\n      \"FirstName\": \"Brett\",\n      \"LastName\": \"Henderson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"fb55731c-befd-47bd-90af-d6c1e0b8d427\",\n      \"FirstName\": \"Brett\",\n      \"LastName\": \"Uglow\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"3974e30f-b14b-4640-9c68-cc0ad710c43e\",\n      \"FirstName\": \"Caroline\",\n      \"LastName\": \"Davis\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"da0b7f5b-b90e-46e6-a79e-4d0b71699f42\",\n      \"FirstName\": \"Cathy\",\n      \"LastName\": \"Jamshidi\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"df49f267-e5a3-4283-bd19-f8c0a9a55a0a\",\n      \"FirstName\": \"Chandan\",\n      \"LastName\": \"Rai\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4f184075-d229-4b12-aecd-a9546f9ab666\",\n      \"FirstName\": \"Chee\",\n      \"LastName\": \"Ding\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"50c15183-cc72-47f7-9f5c-22195e91d6e3\",\n      \"FirstName\": \"Chris\",\n      \"LastName\": \"Carroll\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f5b26f67-a254-46e3-9772-d468656e3206\",\n      \"FirstName\": \"Daniel\",\n      \"MiddleNames\": \"Roy\",\n      \"LastName\": \"Proud\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0bbd0deb-d45a-4f36-aa5d-b9e8bf3853db\",\n      \"FirstName\": \"Daniel\",\n      \"LastName\": \"Mills\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0ee03e63-dae2-4947-97d9-a635633dd8e2\",\n      \"FirstName\": \"Daniel\",\n      \"LastName\": \"Cross\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ad98fc73-558d-4e18-941f-491b5092ffe1\",\n      \"FirstName\": \"Declan\",\n      \"LastName\": \"Robinson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"c20f3f33-c26b-4cfd-9f08-ea0903df1b47\",\n      \"FirstName\": \"Deepa\",\n      \"LastName\": \"Aravindan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"243eeae5-d5e3-4def-a371-e036e6d48e89\",\n      \"FirstName\": \"Dmitry\",\n      \"LastName\": \"Likane\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"06bedb0d-4031-4a8d-a956-c78ae324a24e\",\n      \"FirstName\": \"Don\",\n      \"LastName\": \"Wanniarachchi\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"e5ebeb7a-8cd1-4fdd-8715-dc0d313dcd64\",\n      \"FirstName\": \"Donovan\",\n      \"LastName\": \"Chong\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"e9a245eb-d0eb-40d0-a2a3-91d67a058b6a\",\n      \"FirstName\": \"Eduanne\",\n      \"LastName\": \"Nel\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f09e5a00-5de7-4c0b-96bc-935e1521b4da\",\n      \"FirstName\": \"Edwin\",\n      \"LastName\": \"Granados\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"46325177-3634-492c-843e-4535cc0f47a5\",\n      \"FirstName\": \"Elliott\",\n      \"LastName\": \"Brooks-Miller\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d98d0d12-ccd5-4e67-b1ab-e1c208f06da7\",\n      \"FirstName\": \"Emma\",\n      \"MiddleNames\": \"Carole\",\n      \"LastName\": \"Cullen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"c9de3eb1-e08b-4bc2-8874-cd5d711f2943\",\n      \"FirstName\": \"Erwin\",\n      \"MiddleNames\": \"Rommel\",\n      \"LastName\": \"Acoba\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"17290f76-fdd2-44bc-9192-bd79f96be1f3\",\n      \"FirstName\": \"Gary\",\n      \"LastName\": \"Chang\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d00ada90-30b8-4521-9aed-9749c2e8a579\",\n      \"FirstName\": \"Gawri \",\n      \"LastName\": \"Edussuriya\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4918bea3-c9da-4f0a-b55b-80f8fd315efd\",\n      \"FirstName\": \"Gayan\",\n      \"LastName\": \"Belpamulle\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"167a8ac2-16ed-40ba-82f2-d365a5b5e5ef\",\n      \"FirstName\": \"Ge\",\n      \"LastName\": \"Gao\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4993ea9c-a010-4c38-8199-bb4f7250c0bc\",\n      \"FirstName\": \"Geoffrey\",\n      \"LastName\": \"Harrison\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a4d9d44e-a647-4f90-a788-3ab9d8bfcd3d\",\n      \"FirstName\": \"Grant\",\n      \"MiddleNames\": \"Driver\",\n      \"LastName\": \"Sutton\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"2e50f561-3c55-4d30-9b9d-26c0b84ae0de\",\n      \"FirstName\": \"Gregory\",\n      \"LastName\": \"Austin\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d7c5bb40-2efb-4015-8941-b2e65c03e2a1\",\n      \"FirstName\": \"Harinie\",\n      \"LastName\": \"Immanuel\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"1e72e7e1-dfc9-4fe8-aacd-691247b4b37d\",\n      \"FirstName\": \"Hendrik\",\n      \"LastName\": \"Brandt\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"7104dbd6-91b9-457e-bcd0-402e142e86e2\",\n      \"FirstName\": \"Hojun\",\n      \"LastName\": \"Joo\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"518a87f9-00d5-4a4b-863a-abf230e3dacc\",\n      \"FirstName\": \"Ivan\",\n      \"LastName\": \"Luong\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"419a369a-9997-4420-a199-b31523d57e97\",\n      \"FirstName\": \"Jacob\",\n      \"LastName\": \"Gitlin\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"4f65738f-65d4-4c69-9d01-913044fc6ad1\",\n      \"FirstName\": \"Jake\",\n      \"LastName\": \"Nelson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"422d30a0-819f-48ad-b9f2-8a16fb8cc87a\",\n      \"FirstName\": \"James\",\n      \"LastName\": \"Kieltyka\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"7a193ff8-1f22-4182-b217-1e745810ff9b\",\n      \"FirstName\": \"James\",\n      \"LastName\": \"Martin\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d889e3c2-fd68-42f3-916c-c20c52aed627\",\n      \"FirstName\": \"Jane\",\n      \"LastName\": \"Nguyen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"af72da76-bf60-42bc-a469-8f6cbc92617a\",\n      \"FirstName\": \"Jason\",\n      \"LastName\": \"Feng\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d936ff3a-e849-47d7-a9d6-593e05afeaae\",\n      \"FirstName\": \"Jesse\",\n      \"LastName\": \"Jackson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"bea33f6f-55eb-4459-8a96-7dbe825d4add\",\n      \"FirstName\": \"Jessica\",\n      \"LastName\": \"Odri\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"eb863b11-152f-4b97-b51e-721b8c5f4a97\",\n      \"FirstName\": \"Jiri\",\n      \"LastName\": \"Sklenar\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a6222418-3070-46e8-889e-c78cad4f72f7\",\n      \"FirstName\": \"John\",\n      \"LastName\": \"Stableford\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ac3b4496-2f27-47d6-a0bf-83ba7cf87394\",\n      \"FirstName\": \"John Paul\",\n      \"LastName\": \"Millan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ba6cb47f-e1da-4d6f-93d8-8675ac4c4ebc\",\n      \"FirstName\": \"John-Paul\",\n      \"LastName\": \"Kelly\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"88f9d3de-84ee-4d20-878c-5c76db920464\",\n      \"FirstName\": \"Jonathan\",\n      \"LastName\": \"Derham\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0363b3e4-6fff-45fb-a002-043e3dd10575\",\n      \"FirstName\": \"Jonathon\",\n      \"LastName\": \"Ashworth\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d22a361f-0592-4004-9ea2-e4a4357b0d15\",\n      \"FirstName\": \"Joseph\",\n      \"LastName\": \"Motha\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"edd5bab2-d5a8-452e-8b8a-761a48689e0a\",\n      \"FirstName\": \"Karthik\",\n      \"LastName\": \"Kunjithapatham\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b821a921-0268-4e69-af79-1b90803d3ee2\",\n      \"FirstName\": \"Kelly\",\n      \"LastName\": \"Stewart\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"ee3c2a78-3995-451e-b10f-621ae83f62b1\",\n      \"FirstName\": \"Keren\",\n      \"LastName\": \"Burshtein\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"580ee69f-76fd-4b22-8cb8-c7038e27135e\",\n      \"FirstName\": \"Kinshuk\",\n      \"LastName\": \"Sen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"62448041-200a-4c1e-bdc3-8bd43541d231\",\n      \"FirstName\": \"Kishore\",\n      \"LastName\": \"Nanduri\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"074addea-77e6-4635-8825-44e67561a297\",\n      \"FirstName\": \"Kseniia\",\n      \"LastName\": \"Isaeva\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d33bfdfc-aa4e-4312-9cb3-fd18218e7d2d\",\n      \"FirstName\": \"Laurence\",\n      \"LastName\": \"Judge\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"daee3bd7-bb57-4d8b-aaac-3678f69a6d00\",\n      \"FirstName\": \"Lily\",\n      \"LastName\": \"Huang\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"8d7dd435-9644-43a1-aff9-c194629f78c0\",\n      \"FirstName\": \"Linda\",\n      \"LastName\": \"Connolly\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f699b69a-b980-49e7-b964-e0f29418b2c4\",\n      \"FirstName\": \"Lindsey\",\n      \"LastName\": \"Teal\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"bfd2d710-68af-4a21-83e8-dd5b6575501a\",\n      \"FirstName\": \"Marcel\",\n      \"LastName\": \"McFall\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"23b5677f-d56f-4103-87af-15a7191d82e1\",\n      \"FirstName\": \"Maria\",\n      \"LastName\": \"La Porta\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"a5b97086-61fb-4c60-afe7-d656192d2005\",\n      \"FirstName\": \"Mark\",\n      \"LastName\": \"Sedrak\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"438d3827-655d-435f-9d1a-169901fc1670\",\n      \"FirstName\": \"Mark\",\n      \"LastName\": \"Georgeson\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"debaaf38-4ee7-4c79-9839-9a1de9a2e1f9\",\n      \"FirstName\": \"Matthew\",\n      \"MiddleNames\": \"James\",\n      \"LastName\": \"Corrigan\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"80b12c7f-b8d9-45f6-a4c1-083e7583787e\",\n      \"FirstName\": \"Matthieu\",\n      \"LastName\": \"Siggen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"0c0b5f5d-62da-4228-9c94-d8a0ea0c70e3\",\n      \"FirstName\": \"Michael\",\n      \"LastName\": \"Nguyen\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"13dd1889-35e1-4820-8510-794a34a7b2de\",\n      \"FirstName\": \"Michiel\",\n      \"LastName\": \"Kalkman\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"032b2aa8-14be-47a9-b7c9-0f54232502ad\",\n      \"FirstName\": \"Min Jin\",\n      \"LastName\": \"Tsai\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b47b1f60-159a-474b-a9ba-646998ab8550\",\n      \"FirstName\": \"Mitchell\",\n      \"LastName\": \"Davis\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d225bf03-533a-47b4-9cf1-603a8b4b1113\",\n      \"FirstName\": \"Nick\",\n      \"LastName\": \"Burnard\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"11780b0b-3991-4052-9a87-1222f1c43e86\",\n      \"FirstName\": \"Nick\",\n      \"LastName\": \"Beagley\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"d4d3081c-e7d6-4be8-b98c-3bee07bdb513\",\n      \"FirstName\": \"Nick\",\n      \"LastName\": \"Schnelle\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"85a8a4ab-7641-4f9c-b993-da7b4fb98645\",\n      \"FirstName\": \"Nina\",\n      \"LastName\": \"Zivkovic\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"7ed078c9-670d-4d1d-87cd-e7322a73fd3c\",\n      \"FirstName\": \"Patrick\",\n      \"LastName\": \"Eckel\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"2e19fed8-40f5-418c-ad55-764b2a182e98\",\n      \"FirstName\": \"Peter\",\n      \"LastName\": \"Condick\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"6eb64e85-912d-4af2-b0e8-af3a8db7ac85\",\n      \"FirstName\": \"Peter\",\n      \"LastName\": \"Hall\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"b97a8312-fa79-4468-b7ba-009a16cde3f7\",\n      \"FirstName\": \"Priyanka\",\n      \"LastName\": \"Jagga\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"13bccec3-e844-47ef-a183-82db9875cb08\",\n      \"FirstName\": \"Rajan\",\n      \"LastName\": \"Arkenbout\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"f0cd324b-7705-463e-92f6-b120bd4e10d0\",\n      \"FirstName\": \"Rambabu\",\n      \"LastName\": \"Potla\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"151d4370-b2ba-4fa6-92f5-c2db0d98c8e8\",\n      \"FirstName\": \"Roman\",\n      \"LastName\": \"Makosiy\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"3f96f30e-8b4b-467b-9abc-af758a08c7e0\",\n      \"FirstName\": \"Roman\",\n      \"LastName\": \"Gurevitch\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    },\n    {\n      \"EmployeeID\": \"bb92f7bf-6fc8-4c66-b804-b6d2148ce9de\",\n      \"FirstName\": \"Sam\",\n      \"LastName\": \"McLeod\",\n      \"Status\": \"ACTIVE\",\n      \"PayrollCalendarID\": \"e1a10c07-9d3a-42f3-95d4-7d30a73f1994\"\n    }\n  ]\n}"
		response := &xero.EmpResponse{RateLimitRemaining: 60}
		if err := json.Unmarshal([]byte(empJSON), response); err != nil {
			assert.Failf(t, "There was an error un marshalling the xero API resp", fmt.Sprintf("Error details %v", err))
		}

		leaveBalResp := &xero.LeaveBalanceResponse{Employees: empResp.Employees, RateLimitRemaining: 60}
		r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", "http://dummy", "testEndpoint"), nil)
		require.NoError(t, err)
		mockReqPageOne := &xero.ReusableRequest{Request: r}

		rp, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", "http://dummy", "testEndpointPage2"), nil)
		require.NoError(t, err)
		mockReqPageTwo := &xero.ReusableRequest{Request: rp}

		s, err := session.NewSession()
		require.NoError(t, err)
		sesClient := ses.New(s)

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockReqPageOne, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "2").Return(mockReqPageTwo, nil)
		mockClient.On("GetEmployees", context.Background(), mockReqPageOne).Return(response, nil)
		mockClient.On("GetEmployees", context.Background(), mockReqPageTwo).Return(empResp, errors.New("something went wrong"))
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), digIOTenantID).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(nil)

		service := NewService(mockClient, xlsLocation, sesClient, "", "")
		resp := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, resp)
		assert.True(t, contains(resp, "Failed to fetch employees from Xero. Organization: DigIO. "))
	})

	t.Run("Error when ORG is missing in Xero", func(t *testing.T) {
		cResp := []xero.Connection{
			{
				TenantID:   digIOTenantID,
				TenantType: "Org",
				OrgName:    "DigIO",
			},
		}

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(cResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockRequest, nil)
		mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), digIOTenantID).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(nil)
		xlsLocation := getProjectRoot() + "/test/all_org.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		errRes := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, errRes)
		assert.Equal(t, 14, len(errRes))
		assert.True(t, contains(errRes, "Failed to get Organization details from Xero. Organization: Eliiza. "))
		assert.True(t, contains(errRes, "Failed to get Organization details from Xero. Organization: CMD. "))
	})

	t.Run("Error when employee does not have the applied leave type configured in Xero", func(t *testing.T) {
		expectedError := "Leave type Personal/Carer's Leave not found/configured in Xero for Employee: Syril Sadasivan. Organization: DigIO "

		empResp := &xero.EmpResponse{
			Status: "Active",
			Employees: []xero.Employee{
				{
					EmployeeID:        empID,
					FirstName:         "Syril",
					LastName:          "Sadasivan",
					Status:            "Active",
					PayrollCalendarID: "4567891011",
					LeaveBalance: []xero.LeaveBalance{
						annualLeave,
					},
				},
				{
					EmployeeID:        empID,
					FirstName:         "Syril",
					LastName:          "Sadasivan",
					Status:            "Active",
					PayrollCalendarID: "4567891011",
					LeaveBalance: []xero.LeaveBalance{
						annualLeave,
					},
				},
			},
			RateLimitRemaining: 60,
		}

		leaveBalResp := &xero.LeaveBalanceResponse{Employees: empResp.Employees, RateLimitRemaining: 60}

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockRequest, nil)
		mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), digIOTenantID).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(nil)
		xlsLocation := getProjectRoot() + "/test/digio_leave.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		errRes := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, errRes)
		assert.True(t, contains(errRes, expectedError))
	})

	t.Run("Error when employee is missing in Xero", func(t *testing.T) {
		expectedError := "Employee not found in Xero. Employee: Stina Anderson. Organization: Mantel Group"

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), mock.Anything, "1").Return(mockRequest, nil)
		mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), mock.Anything).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), mock.Anything, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), mock.Anything, mock.Anything).Return(nil)

		xlsLocation := getProjectRoot() + "/test/all_org.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		errRes := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, errRes)
		assert.Equal(t, 12, len(errRes))
		assert.True(t, contains(errRes, expectedError))
	})

	t.Run("Error when payroll calendar settings not found for employee", func(t *testing.T) {
		expectedError := "Failed to fetch employee payroll calendar settings from Xero. Employee: Stina Anderson. Organization: CMD "

		empResp := &xero.EmpResponse{
			Status: "Active",
			Employees: []xero.Employee{
				{
					EmployeeID:        empID,
					FirstName:         "Syril",
					LastName:          "Sadasivan",
					Status:            "Active",
					PayrollCalendarID: "4567891011",
					LeaveBalance: []xero.LeaveBalance{
						annualLeave,
						personalLeave,
					},
				},
				{
					EmployeeID:        "45678974111",
					FirstName:         "Stina",
					LastName:          "Anderson",
					Status:            "Active",
					PayrollCalendarID: "789845651232",
					LeaveBalance: []xero.LeaveBalance{
						annualLeave,
						personalLeave,
					},
				},
			},
			RateLimitRemaining: 60,
		}

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), mock.Anything, "1").Return(mockRequest, nil)
		mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(payRollCalendarResp, nil)
		mockClient.On("NewPayrollRequest", context.Background(), mock.Anything).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), mock.Anything, empID).Return(leaveBalResp, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), mock.Anything, mock.Anything).Return(nil)

		xlsLocation := getProjectRoot() + "/test/cmd_leave.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		errRes := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, errRes)
		assert.Equal(t, 1, len(errRes))
		assert.True(t, contains(errRes, expectedError))
	})

	t.Run("Error when failed to post leave request to xero", func(t *testing.T) {
		expectedError := "Failed to post Leave application to xero for Employee: Syril Sadasivan Organization: DigIO"

		digIOEmpResp := &xero.EmpResponse{
			Status: "Active",
			Employees: []xero.Employee{
				{
					EmployeeID:        empID,
					FirstName:         "Syril",
					LastName:          "Sadasivan",
					Status:            "Active",
					PayrollCalendarID: "4567891011",
					LeaveBalance: []xero.LeaveBalance{
						annualLeave,
						personalLeave,
					},
				},
			},
			RateLimitRemaining: 60,
		}

		digIOLeaveBal := &xero.LeaveBalanceResponse{Employees: digIOEmpResp.Employees, RateLimitRemaining: 60}
		digIOPayrollCal := &xero.PayrollCalendarResponse{
			PayrollCalendars: []xero.PayrollCalendar{
				{
					PayrollCalendarID: "4567891011",
					PaymentDate:       "/Date(632102400000+0000)/",
				},
			},
		}

		mockClient := new(MockXeroClient)
		mockClient.On("GetConnections", context.Background()).Return(connectionResp, nil)
		mockClient.On("NewGetEmployeesRequest", context.Background(), digIOTenantID, "1").Return(mockRequest, nil)
		mockClient.On("GetEmployees", context.Background(), any(mockRequest)).Return(empResp, nil)
		mockClient.On("GetPayrollCalendars", context.Background(), any(mockRequest)).Return(digIOPayrollCal, nil)
		mockClient.On("NewPayrollRequest", context.Background(), mock.Anything).Return(mockRequest, nil)
		mockClient.On("EmployeeLeaveBalance", context.Background(), digIOTenantID, empID).Return(digIOLeaveBal, nil)
		mockClient.On("EmployeeLeaveApplication", context.Background(), digIOTenantID, mock.Anything).Return(errors.New("something went wrong"))

		xlsLocation := getProjectRoot() + "/test/failed_leave.xlsx"
		service := NewService(mockClient, xlsLocation, sesClient, "", "")

		errRes := service.MigrateLeaveKrowToXero(context.Background())
		assert.NotNil(t, errRes)
		assert.Equal(t, 1, len(errRes))
		assert.True(t, contains(errRes, expectedError))
	})
}

func contains(errors []string, errStr string) bool {
	for _, s := range errors {
		if strings.Contains(s, errStr) {
			return true
		}
	}
	return false
}

func getProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b)
	dir := path.Join(path.Dir(basePath), ".")
	return dir
}

func (m *MockXeroClient) GetConnections(ctx context.Context) ([]xero.Connection, error) {
	args := m.Called(ctx)
	return args.Get(0).([]xero.Connection), args.Error(1)
}

func (m *MockXeroClient) EmployeeLeaveBalance(ctx context.Context, tenantID string, empID string) (*xero.LeaveBalanceResponse, error) {
	args := m.Called(ctx, tenantID, empID)
	return args.Get(0).(*xero.LeaveBalanceResponse), args.Error(1)
}

func (m *MockXeroClient) EmployeeLeaveApplication(ctx context.Context, tenantID string, request xero.LeaveApplicationRequest) error {
	args := m.Called(ctx, tenantID, request)
	return args.Error(0)
}

func (m *MockXeroClient) GetPayrollCalendars(ctx context.Context, req *xero.ReusableRequest) (*xero.PayrollCalendarResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*xero.PayrollCalendarResponse), args.Error(1)
}

func (m *MockXeroClient) NewPayrollRequest(ctx context.Context, tenantID string) (*xero.ReusableRequest, error) {
	args := m.Called(ctx, tenantID)
	return args.Get(0).(*xero.ReusableRequest), args.Error(1)
}

func (m *MockXeroClient) GetEmployees(ctx context.Context, req *xero.ReusableRequest) (*xero.EmpResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*xero.EmpResponse), args.Error(1)
}

func (m *MockXeroClient) NewGetEmployeesRequest(ctx context.Context, tenantID string, page string) (*xero.ReusableRequest, error) {
	args := m.Called(ctx, tenantID, page)
	return args.Get(0).(*xero.ReusableRequest), args.Error(1)
}
