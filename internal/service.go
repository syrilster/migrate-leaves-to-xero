package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"gopkg.in/gomail.v2"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	log "github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"

	"github.com/syrilster/migrate-leave-krow-to-xero/internal/model"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/xero"
)

var minRateLimit = 60

const (
	unPaidLeave        string = "Other Unpaid Leave"
	compassionateLeave string = "Compassionate Leave (paid)"
	juryDutyLeave      string = "Jury Duty"
	personalLeave      string = "Personal/Carer's Leave"
	annualLeave        string = "Annual Leave"

	annualLeaveNegativeLimit   float64 = -40
	personalLeaveNegativeLimit float64 = -16
)

type Service struct {
	client          xero.ClientInterface
	xlsFileLocation string
	emailClient     *ses.SES
	emailTo         string
	emailFrom       string
}

type EmpLeaveRequest struct {
	empID             string
	empName           string
	tenantID          string
	leaveTypeID       string
	leaveUnits        float64
	paymentDate       string
	leaveStartDate    string
	leaveEndDate      string
	leaveType         string
	leaveDate         time.Time
	originalLeaveType string
	orgName           string
	description       string
}

func NewService(c xero.ClientInterface, xlsLocation string, ec *ses.SES, emailTo string, emailFrom string) *Service {
	return &Service{
		client:          c,
		xlsFileLocation: xlsLocation,
		emailClient:     ec,
		emailTo:         emailTo,
		emailFrom:       emailFrom,
	}
}

//MigrateLeaveKrowToXero func will process the leave requests
func (service Service) MigrateLeaveKrowToXero(ctx context.Context) []string {
	var errResult []string
	var successResult []string
	var errStrings []error
	var wg sync.WaitGroup
	var xeroEmployeesMap map[string]xero.Employee
	var payrollCalendarMap = make(map[string]string)
	var connectionsMap = make(map[string]string)
	var resultChan = make(chan string)
	var orgEmpCacheList []string
	var payrollCalCacheList []string

	ctxLogger := log.WithContext(ctx)
	ctxLogger.Infof("Executing MigrateLeaveKrowToXero service")

	xeroEmployeesMap = make(map[string]xero.Employee)
	leaveRequests, errResult := service.extractDataFromKrow(ctx, errResult)
	if len(errResult) > 0 {
		ctxLogger.Infof("There were %v errors during extracting excel data", len(errResult))
	}
	ctxLogger.Info("Leave Requests length: ", len(leaveRequests))

	if len(leaveRequests) == 0 {
		service.sendStatusReport(ctx, errResult, successResult)
		return errResult
	}

	ctxLogger.Info("Processing Leave Requests")
	resp, err := service.client.GetConnections(ctx)
	if err != nil {
		errStr := fmt.Errorf("Failed to fetch connections from Xero: %v. Please try again later or contact admin. ", err)
		ctxLogger.Infof(errStr.Error())
		errResult = append(errResult, errStr.Error())
		service.sendStatusReport(ctx, errResult, successResult)
		return errResult
	}

	for _, c := range resp {
		connectionsMap[c.OrgName] = c.TenantID
	}

	for _, leaveReq := range leaveRequests {
		//To avoid Xero Minute Limit: 60 calls per minute
		if minRateLimit < 5 {
			ctxLogger.Info("Pausing the APP run due to less rate limit. Remaining: ", minRateLimit)
			time.Sleep(60 * time.Second)
		}

		if _, ok := connectionsMap[leaveReq.OrgName]; !ok {
			errStr := fmt.Errorf("Failed to get Organization details from Xero. Organization: %v. ", leaveReq.OrgName)
			ctxLogger.Infof(errStr.Error())
			errStrings = append(errStrings, errStr)
			continue
		}

		tenantID := connectionsMap[leaveReq.OrgName]
		//Use employees available in the local cache(Map) rather than loading for each leave request. This is to avoid xero rate limit 429 error
		if !containsString(orgEmpCacheList, leaveReq.OrgName) {
			var errs []string
			xeroEmployeesMap, errs = service.populateEmployeesMap(ctx, xeroEmployeesMap, tenantID, leaveReq.OrgName, 1)
			if errs != nil {
				errResult = errs
				continue
			}
			orgEmpCacheList = append(orgEmpCacheList, leaveReq.OrgName)
		}

		if !containsString(payrollCalCacheList, tenantID) {
			req, err := service.client.NewPayrollRequest(ctx, tenantID)
			if err != nil {
				errStr := fmt.Errorf("failed to build NewPayrollRequest. Cause %v", err.Error())
				ctxLogger.Infof(err.Error(), err)
				errStrings = append(errStrings, errStr)
				continue
			}

			payCalendarResp, err := service.client.GetPayrollCalendars(ctx, req)
			if err != nil {
				errStr := fmt.Errorf("Failed to fetch employee payroll calendar settings from Xero. Organization: %v. Please reupload entry for this ORG. ", leaveReq.OrgName)
				ctxLogger.Infof(err.Error(), err)
				errStrings = append(errStrings, errStr)
				continue
			}

			//Populate the payroll settings to a map
			for _, p := range payCalendarResp.PayrollCalendars {
				payrollCalendarMap[p.PayrollCalendarID] = p.PaymentDate
			}

			payrollCalCacheList = append(payrollCalCacheList, tenantID)
		}

		errStr := service.processLeaveRequestByEmp(ctx, xeroEmployeesMap, leaveReq, tenantID, payrollCalendarMap, resultChan, &wg)
		if errStr != nil {
			if !containsError(errStrings, errStr.Error()) {
				errStrings = append(errStrings, errStr)
			}
		}
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for _, e := range errStrings {
		if e.Error() != "" {
			errResult = append(errResult, e.Error())
		}
	}
	for result := range resultChan {
		if strings.Contains(result, "Error:") {
			errResult = append(errResult, result)
		} else {
			successResult = append(successResult, result)
		}
	}

	service.sendStatusReport(ctx, errResult, successResult)
	if len(errResult) > 0 {
		return errResult
	}
	return nil
}

func (service Service) populateEmployeesMap(ctx context.Context, xeroEmployeesMap map[string]xero.Employee, tenantID string, orgName string, page int) (empMap map[string]xero.Employee, errRes []string) {
	ctxLogger := log.WithContext(ctx)
	emptyMap := make(map[string]xero.Employee)
	var errResult []string

	empResponse, err := service.client.GetEmployees(ctx, tenantID, strconv.Itoa(page))
	if err != nil {
		errStr := fmt.Errorf("Failed to fetch employees from Xero. Organization: %v. ", orgName)
		ctxLogger.Infof(err.Error(), err)
		errResult = append(errResult, errStr.Error())
		return emptyMap, errResult
	}

	minRateLimit = empResponse.RateLimitRemaining
	//populate the employees to a map
	for _, emp := range empResponse.Employees {
		xeroEmployeesMap[emp.FirstName+" "+emp.LastName] = emp
	}

	//Recursive call to get next page
	if len(empResponse.Employees) > 99 {
		var errs []string
		xeroEmployeesMap, errs = service.populateEmployeesMap(ctx, xeroEmployeesMap, tenantID, orgName, page+1)
		if errs != nil {
			errResult = errs
			return emptyMap, errs
		}
	}

	return xeroEmployeesMap, nil
}

func (service Service) sendStatusReport(ctx context.Context, errResult []string, result []string) {
	resultString := strings.Join(result, "\n")
	errorsString := strings.Join(errResult, "\n")
	if errorsString == "" {
		errorsString = "No errors found during processing leaves. Please check attached report for audit trail."
	}
	go service.sesSendEmail(ctx, resultString, errorsString)
}

func (service Service) processLeaveRequestByEmp(ctx context.Context, xeroEmployeesMap map[string]xero.Employee,
	leaveReq model.KrowLeaveRequest, tenantID string, payrollCalendarMap map[string]string,
	resChan chan string, wg *sync.WaitGroup) error {
	ctxLogger := log.WithContext(ctx)

	if _, ok := xeroEmployeesMap[leaveReq.EmpName]; !ok {
		errStr := fmt.Errorf("Employee not found in Xero. Employee: %v. Organization: %v  ", leaveReq.EmpName, leaveReq.OrgName)
		ctxLogger.Infof(errStr.Error())
		return errStr
	}

	empID := xeroEmployeesMap[leaveReq.EmpName].EmployeeID
	payCalendarID := xeroEmployeesMap[leaveReq.EmpName].PayrollCalendarID
	if _, ok := payrollCalendarMap[payCalendarID]; !ok {
		errStr := fmt.Errorf("Failed to fetch employee payroll calendar settings from Xero. Employee: %v. Organization: %v ", leaveReq.EmpName, leaveReq.OrgName)
		ctxLogger.Infof(errStr.Error())
		return errStr
	}

	paymentDate := payrollCalendarMap[payCalendarID]
	err := service.reconcileLeaveRequestAndApply(ctx, empID, tenantID, leaveReq, paymentDate, resChan, wg)
	return err
}

func (service Service) reconcileLeaveRequestAndApply(ctx context.Context, empID string, tenantID string,
	leaveReq model.KrowLeaveRequest, paymentDate string, resChan chan string, wg *sync.WaitGroup) error {
	var leaveBalanceMap = make(map[string]xero.LeaveBalance)
	var leaveTypeID string
	var leaveStartDate string
	var leaveEndDate string
	var unpaidLeaveUnits float64
	var paidLeaveUnits float64
	var unPaidLeaveTypeID string
	var errorsStr []string
	var skipUnpaidLeave bool
	var negativeLeaveLimit float64

	ctxLogger := log.WithContext(ctx)
	ctxLogger.Infof("Calculating leaves to be applied for Employee %v", leaveReq.EmpName)

	skipUnpaidLeave = strings.EqualFold(leaveReq.LeaveType, compassionateLeave) || strings.EqualFold(leaveReq.LeaveType, juryDutyLeave)

	//Just to make sure that the previous leave request if any has been completed and we get the updated balance.
	time.Sleep(200 * time.Millisecond)
	leaveBalance, err := service.client.EmployeeLeaveBalance(ctx, tenantID, empID)
	if err != nil {
		errStr := fmt.Errorf("Failed to fetch employee leave balance from Xero. Employee: %v. Organization: %v ", leaveReq.EmpName, leaveReq.OrgName)
		ctxLogger.Infof(errStr.Error(), err)
		return errStr
	}
	minRateLimit = leaveBalance.RateLimitRemaining

	for _, leaveBal := range leaveBalance.Employees[0].LeaveBalance {
		leaveBalanceMap[leaveBal.LeaveType] = leaveBal
		if strings.EqualFold(leaveBal.LeaveType, unPaidLeave) {
			unPaidLeaveTypeID = leaveBal.LeaveTypeID
		}
	}

	if _, ok := leaveBalanceMap[leaveReq.LeaveType]; !ok {
		errStr := fmt.Errorf("Leave type %v not found/configured in Xero for Employee: %v. Organization: %v ", leaveReq.LeaveType, leaveReq.EmpName, leaveReq.OrgName)
		ctxLogger.Infof(errStr.Error())
		errorsStr = append(errorsStr, errStr.Error())
		return errStr
	}

	lb := leaveBalanceMap[leaveReq.LeaveType]
	leaveReqUnit := leaveReq.Hours
	availableLeaveBalUnit := lb.NumberOfUnits
	leaveTypeID = lb.LeaveTypeID
	leaveStartDate = "/Date(" + strconv.FormatInt(leaveReq.LeaveDateEpoch, 10) + ")/"
	leaveEndDate = "/Date(" + strconv.FormatInt(leaveReq.LeaveDateEpoch, 10) + ")/"
	//Special case for annual leave and personal leave i.e negative leave allowed
	if strings.EqualFold(leaveReq.LeaveType, annualLeave) || strings.EqualFold(leaveReq.LeaveType, personalLeave) {
		if strings.EqualFold(leaveReq.LeaveType, personalLeave) {
			negativeLeaveLimit = personalLeaveNegativeLimit
		} else {
			negativeLeaveLimit = annualLeaveNegativeLimit
		}
		//To handle an edge case if leave is for ex: -44 then reset to zero
		if availableLeaveBalUnit < negativeLeaveLimit {
			availableLeaveBalUnit = 0
		} else if availableLeaveBalUnit > 0 {
			//To handle a case when leave is positive for ex:20
			availableLeaveBalUnit = math.Abs(negativeLeaveLimit) + availableLeaveBalUnit
		} else {
			//leave already in negative
			availableLeaveBalUnit = math.Abs(negativeLeaveLimit - availableLeaveBalUnit)
		}
	}
	if leaveReqUnit >= availableLeaveBalUnit {
		if availableLeaveBalUnit > 0 {
			paidLeaveUnits = availableLeaveBalUnit
			unpaidLeaveUnits += leaveReqUnit - availableLeaveBalUnit
		} else {
			//Employee has negative or zero leave balance and hence unpaid leave
			paidLeaveUnits = 0
			unpaidLeaveUnits += leaveReqUnit
		}
	} else {
		paidLeaveUnits = leaveReqUnit
	}

	if paidLeaveUnits > 0 {
		wg.Add(1)
		paidLeaveReq := EmpLeaveRequest{
			empID:             empID,
			empName:           leaveReq.EmpName,
			tenantID:          tenantID,
			leaveTypeID:       leaveTypeID,
			leaveUnits:        paidLeaveUnits,
			paymentDate:       paymentDate,
			leaveStartDate:    leaveStartDate,
			leaveEndDate:      leaveEndDate,
			leaveType:         leaveReq.LeaveType,
			leaveDate:         leaveReq.LeaveDate,
			originalLeaveType: leaveReq.LeaveType,
			orgName:           leaveReq.OrgName,
			description:       leaveReq.Description,
		}
		service.applyLeave(ctx, paidLeaveReq, resChan, wg)
	}

	if unpaidLeaveUnits > 0 && !skipUnpaidLeave {
		wg.Add(1)
		unPaidLeaveReq := EmpLeaveRequest{
			empID:             empID,
			empName:           leaveReq.EmpName,
			tenantID:          tenantID,
			leaveTypeID:       unPaidLeaveTypeID,
			leaveUnits:        unpaidLeaveUnits,
			paymentDate:       paymentDate,
			leaveStartDate:    leaveStartDate,
			leaveEndDate:      leaveEndDate,
			leaveType:         unPaidLeave,
			leaveDate:         leaveReq.LeaveDate,
			originalLeaveType: leaveReq.LeaveType,
			orgName:           leaveReq.OrgName,
			description:       leaveReq.Description,
		}
		service.applyLeave(ctx, unPaidLeaveReq, resChan, wg)
	}

	if unpaidLeaveUnits > 0 && skipUnpaidLeave {
		errStr := fmt.Errorf("Employee: %v has insufficient Leave balance for Leave type %v requested for %v hours ", leaveReq.EmpName, leaveReq.LeaveType, unpaidLeaveUnits)
		errorsStr = append(errorsStr, errStr.Error())
	}

	e := strings.Join(errorsStr, "\n")
	errRes := errors.New(e)
	return errRes
}

func (service Service) applyLeave(ctx context.Context, leaveReq EmpLeaveRequest, resChan chan string, wg *sync.WaitGroup) {
	var leavePeriods = make([]xero.LeavePeriod, 1)
	leavePeriod := xero.LeavePeriod{
		PayPeriodEndDate: leaveReq.paymentDate,
		NumberOfUnits:    leaveReq.leaveUnits,
	}
	leaveDate := leaveReq.leaveDate.Format("2/1/2006")
	leavePeriods[0] = leavePeriod

	if leaveReq.description == "" {
		leaveReq.description = leaveReq.leaveType + " " + leaveReq.leaveDate.Format("02/01")
	}

	leaveApplication := xero.LeaveApplicationRequest{
		EmployeeID:   leaveReq.empID,
		LeaveTypeID:  leaveReq.leaveTypeID,
		StartDate:    leaveReq.leaveStartDate,
		EndDate:      leaveReq.leaveEndDate,
		Title:        leaveReq.description,
		LeavePeriods: leavePeriods,
	}
	go service.applyLeaveRequestToXero(ctx, leaveReq.tenantID, leaveReq.leaveType, leaveReq.originalLeaveType,
		leaveDate, leaveApplication, leaveReq.empName, leaveReq.orgName, resChan, wg)
}

func (service Service) applyLeaveRequestToXero(ctx context.Context, tenantID string, appliedLeaveType string, originalLeaveType string,
	leaveDate string, leaveApplication xero.LeaveApplicationRequest, empName string, orgName string, resChan chan string, wg *sync.WaitGroup) {
	ctxLogger := log.WithContext(ctx)
	ctxLogger.Infof("Applying leave request for Employees: %v", empName)

	defer func() {
		wg.Done()
	}()

	err := service.client.EmployeeLeaveApplication(ctx, tenantID, leaveApplication)
	if err != nil {
		ctxLogger.Infof("Leave Application Request: %v", leaveApplication)
		ctxLogger.WithError(err).Errorf("Failed to post Leave application to xero for Employee: %v Organization: %v", empName, orgName)
		resChan <- fmt.Sprintf("Error: Failed to post Leave application to xero for Employee: %v Organization: %v ", empName, orgName)
		return
	}
	resChan <- fmt.Sprintf("%v,%v,%v,%v,%v,%v",
		empName, originalLeaveType, appliedLeaveType, leaveDate, leaveApplication.LeavePeriods[0].NumberOfUnits, orgName)
}

func (service Service) extractDataFromKrow(ctx context.Context, errResult []string) ([]model.KrowLeaveRequest, []string) {
	var leaveRequests []model.KrowLeaveRequest
	ctxLogger := log.WithContext(ctx)

	f, err := excelize.OpenFile(service.xlsFileLocation)
	if err != nil {
		errStr := fmt.Errorf("Unable to open the uploaded file. Please confirm the file is in xlsx format. ")
		ctxLogger.WithError(err).Error(errStr)
		errResult = append(errResult, errStr.Error())
		return nil, errResult
	}

	ctxLogger.Info("SheetName: ", f.GetSheetName(f.GetActiveSheetIndex()))
	rows, err := f.GetRows(f.GetSheetName(f.GetActiveSheetIndex()), excelize.Options{RawCellValue: true})
	for index, row := range rows {
		// This is to skip the header row of the excel sheet
		if index == 0 {
			continue
		}

		rawDate := row[1]
		ld, err := strconv.ParseFloat(rawDate, 64)
		leaveDate, err := excelize.ExcelDateToTime(ld, false)
		if err != nil || dateContainsSpecialChars(rawDate) {
			errStr := fmt.Errorf("Invalid entry for Leave Date: %v. Valid Format DD/MM/YYYY (Ex: 01/06/2020)", rawDate)
			if err != nil {
				ctxLogger.WithError(err).Error(errStr)
			}
			errResult = append(errResult, errStr.Error())
			continue
		}

		hours, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			errStr := fmt.Errorf("Invalid entry for Leave Hours: %v ", row[2])
			ctxLogger.WithError(err).Error(errStr)
			errResult = append(errResult, errStr.Error())
			continue
		}

		leaveType := row[3]
		if leaveType == "" {
			leaveType = row[4]
		}

		r := strings.NewReplacer("Carers", "Carer's",
			"Unpaid", "Other Unpaid",
			"Parental Leave (10 days for new family member)", "Parental Leave (Paid)",
			"Parental Leave", "Parental Leave (Paid)",
			"Compassionate Leave", "Compassionate Leave (paid)")
		leaveType = r.Replace(leaveType)
		empName := row[0]
		orgName := row[5]
		o := strings.NewReplacer("Cuusoo", "Cuusoo Pty Ltd")
		org := o.Replace(orgName)
		desc := ""
		// this means that there is a description column
		if len(row) == 7 {
			desc = row[6]
		}

		leaveReq := model.KrowLeaveRequest{
			LeaveDate:      leaveDate,
			LeaveDateEpoch: leaveDate.UnixNano() / 1000000,
			Hours:          hours,
			LeaveType:      leaveType,
			OrgName:        org,
			EmpName:        empName,
			Description:    desc,
		}
		leaveRequests = append(leaveRequests, leaveReq)
	}
	return leaveRequests, errResult
}

func (service Service) sesSendEmail(ctx context.Context, attachmentData string, data string) {
	contextLogger := log.WithContext(ctx)
	contextLogger.Infof("Inside sesSendEmail func")
	attachFileName := "/tmp/report.xlsx"

	writeAttachmentDataToExcel(ctx, attachFileName, attachmentData)

	msg := gomail.NewMessage()
	msg.SetHeader("From", service.emailFrom)
	msg.SetHeader("To", service.emailTo)
	msg.SetHeader("Subject", "Report: Leave Migration to Xero")
	msg.SetBody("text/plain", data)
	msg.Attach(attachFileName)

	var emailRaw bytes.Buffer
	_, err := msg.WriteTo(&emailRaw)
	if err != nil {
		contextLogger.WithError(err).Error("Error when writing email data")
		return
	}

	message := ses.RawMessage{Data: emailRaw.Bytes()}
	recipients := populateEmailRecipients(service.emailTo)
	emailParams := ses.SendRawEmailInput{
		Source:     aws.String(service.emailFrom),
		RawMessage: &message,
	}
	emailParams.SetDestinations(recipients)

	_, err = service.emailClient.SendRawEmail(&emailParams)
	if err != nil {
		contextLogger.WithError(err).Error("Error when sending email")
		return
	}
	contextLogger.Infof("Finished sesSendEmail func")
	return
}

func populateEmailRecipients(emailTo string) []*string {
	var emailRecipients []*string
	recipients := strings.Split(emailTo, ",")
	for _, recipient := range recipients {
		emailRecipients = append(emailRecipients, aws.String(recipient))
	}
	return emailRecipients
}

func writeAttachmentDataToExcel(ctx context.Context, attachFileName string, attachmentData string) {
	contextLogger := log.WithContext(ctx)
	f := excelize.NewFile()
	// Create a new sheet.
	index := f.NewSheet("Sheet1")
	_ = f.SetColWidth("Sheet1", "A", "E", 20)
	_ = f.SetColWidth("Sheet1", "B", "C", 30)
	// Set value of a cell.
	err := f.SetCellValue("Sheet1", "A1", "Employee")
	if err != nil {
		contextLogger.WithError(err)
		return
	}
	err = f.SetCellValue("Sheet1", "B1", "Leave Requested")
	if err != nil {
		contextLogger.WithError(err)
		return
	}
	err = f.SetCellValue("Sheet1", "C1", "Leave Applied (Xero)")
	if err != nil {
		contextLogger.WithError(err)
		return
	}
	err = f.SetCellValue("Sheet1", "D1", "Leave Date")
	if err != nil {
		contextLogger.WithError(err)
		return
	}
	err = f.SetCellValue("Sheet1", "E1", "Hours")
	if err != nil {
		contextLogger.WithError(err)
		return
	}
	err = f.SetCellValue("Sheet1", "F1", "Org")
	if err != nil {
		contextLogger.WithError(err)
		return
	}

	if len(attachmentData) > 0 {
		rows := strings.Split(attachmentData, "\n")
		rowStartIndex := 2
		for _, row := range rows {
			cells := strings.Split(row, ",")
			if len(cells) > 0 {
				rowStartIndexStr := strconv.Itoa(rowStartIndex)
				// Cell style related
				normalStyle, err := f.NewStyle(`{"font":{"bold":false, "family":"Liberation Serif"}}`)
				if err != nil {
					contextLogger.WithError(err).Errorf("Unable to create column style")
					return
				}
				boldStyle, err := f.NewStyle(`{"font":{"color":"#FF0000", "bold":true, "family":"Liberation Serif"}}`)
				if err != nil {
					contextLogger.WithError(err).Errorf("Unable to create column style")
					return
				}
				style := normalStyle

				leaveReq := cells[1]
				leaveApplied := cells[2]
				if leaveReq != leaveApplied {
					style = boldStyle
				}

				err = f.SetCellValue("Sheet1", "A"+rowStartIndexStr, cells[0])
				if err != nil {
					contextLogger.WithError(err)
					return
				}
				err = f.SetCellStyle("Sheet1", "B"+rowStartIndexStr, "B"+rowStartIndexStr, style)
				if err != nil {
					contextLogger.WithError(err).Errorf("Unable to set cell style")
					return
				}
				err = f.SetCellValue("Sheet1", "B"+rowStartIndexStr, cells[1])
				if err != nil {
					contextLogger.WithError(err)
					return
				}
				err = f.SetCellStyle("Sheet1", "C"+rowStartIndexStr, "C"+rowStartIndexStr, style)
				if err != nil {
					contextLogger.WithError(err).Errorf("Unable to set cell style")
					return
				}
				err = f.SetCellValue("Sheet1", "C"+rowStartIndexStr, cells[2])
				if err != nil {
					contextLogger.WithError(err)
					return
				}
				err = f.SetCellValue("Sheet1", "D"+rowStartIndexStr, cells[3])
				if err != nil {
					contextLogger.WithError(err)
					return
				}
				err = f.SetCellValue("Sheet1", "E"+rowStartIndexStr, cells[4])
				if err != nil {
					contextLogger.WithError(err)
					return
				}
				err = f.SetCellValue("Sheet1", "F"+rowStartIndexStr, cells[5])
				if err != nil {
					contextLogger.WithError(err)
					return
				}
				rowStartIndex++
			}
		}
	}

	// Set active sheet of the workbook.
	f.SetActiveSheet(index)
	// Save xlsx file by the given path.
	if err := f.SaveAs(attachFileName); err != nil {
		fmt.Println(err)
	}
}

func containsError(errors []error, errStr string) bool {
	for _, s := range errors {
		if strings.Contains(s.Error(), errStr) {
			return true
		}
	}
	return false
}

func containsString(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// dateContainsSpecialChars is a func to check if the leave date contains any special chars
// The raw date from the Excel is supposed to be of the format 43949 for date 28/04/2020. If the
// date is not in this format it will be in either 28/04/2020 or 28-04-2020 which is then considered invalid
func dateContainsSpecialChars(date string) bool {
	return strings.Contains(date, "/") || strings.Contains(date, "-")
}
