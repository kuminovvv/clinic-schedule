package main

// equalDayPlanContents ignores revision and patient row order. The order of
// procedures within one patient remains significant for scheduling.
func equalDayPlanContents(a, b dayPlan) bool {
	if a.Date != b.Date || len(a.Plans) != len(b.Plans) {
		return false
	}
	if (a.SelectedPatientNumber == nil) != (b.SelectedPatientNumber == nil) {
		return false
	}
	if a.SelectedPatientNumber != nil && *a.SelectedPatientNumber != *b.SelectedPatientNumber {
		return false
	}
	byPatient := make(map[int]patientPlan, len(a.Plans))
	for _, patient := range a.Plans {
		byPatient[patient.PatientNumber] = patient
	}
	for _, patient := range b.Plans {
		previous, found := byPatient[patient.PatientNumber]
		if !found || previous.Severity != patient.Severity || previous.StayDays != patient.StayDays ||
			len(previous.ProcedureIDs) != len(patient.ProcedureIDs) || len(previous.ProcedureCounts) != len(patient.ProcedureCounts) {
			return false
		}
		for index, id := range patient.ProcedureIDs {
			if previous.ProcedureIDs[index] != id || previous.ProcedureCounts[id] != patient.ProcedureCounts[id] {
				return false
			}
		}
	}
	return true
}
