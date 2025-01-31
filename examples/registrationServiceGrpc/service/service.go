package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/MarcGrol/go-training/examples/registrationServiceGrpc/lib/api/datastorer"
	"github.com/MarcGrol/go-training/examples/registrationServiceGrpc/lib/api/emailsender"
	"github.com/MarcGrol/go-training/examples/registrationServiceGrpc/lib/api/pincoder"
	"github.com/MarcGrol/go-training/examples/registrationServiceGrpc/lib/api/uuider"
	"github.com/MarcGrol/go-training/examples/registrationServiceGrpc/regprotobuf"
)

const (
	maxAttempts = 5
)

type RegistrationService struct {
	uuidGenerator    uuider.UuidGenerator
	patientStore     datastorer.PatientStorer
	emailSender      emailsender.EmailSender
	pincodeGenerator pincoder.PincodeGenerator
	regprotobuf.UnimplementedRegistrationServiceServer
}

func NewRegistrationService(uuidGenerator uuider.UuidGenerator, patientStore datastorer.PatientStorer, pincoder pincoder.PincodeGenerator,
	emailSender emailsender.EmailSender) *RegistrationService {
	return &RegistrationService{
		uuidGenerator:    uuidGenerator,
		patientStore:     patientStore,
		pincodeGenerator: pincoder,
		emailSender:      emailSender,
	}
}

func (rs *RegistrationService) RegisterPatient(ctx context.Context, req *regprotobuf.RegisterPatientRequest) (*regprotobuf.RegisterPatientResponse, error) {
	err := validateRegisterPatientRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Error validating request: %s", err.Error())
	}

	pincode := rs.pincodeGenerator.GeneratePincode()
	emailSubject := "Registration pincode"
	emailContent := fmt.Sprintf("Finalize registration with pincode %d", pincode)
	err = rs.emailSender.SendEmail(req.Patient.Contact.EmailAddress, emailSubject, emailContent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error sending email: %s", err)
	}

	patient := patientToInternal(req.Patient)
	patient.RegistrationPin = pincode
	patient.UID = rs.uuidGenerator.GenerateUuid()
	patient.RegistrationStatus = datastorer.Pending

	log.Printf("Started registration of user %+v", patient)

	err = rs.patientStore.PutPatientOnUid(patient)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error storring patient: %s", err)
	}

	return &regprotobuf.RegisterPatientResponse{
		PatientUid: patient.UID,
	}, nil
}

func (rs *RegistrationService) CompletePatientRegistration(ctx context.Context, req *regprotobuf.CompletePatientRegistrationRequest) (*regprotobuf.CompletePatientRegistrationResponse, error) {
	err := validatePatientRegistrationRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Error validating input: %s", err)
	}

	// TODO the store.Get and store.Put should run within a transaction

	patient, found, err := rs.patientStore.GetPatientOnUid(req.PatientUid)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error getting patient in uid: %s", err)
	}
	if !found {
		return nil, status.Errorf(codes.NotFound, "Patient with uid not found")
	}

	if patient.RegistrationStatus == datastorer.Blocked {
		return nil, status.Errorf(codes.InvalidArgument, "Patient blocked")
	}

	if int(req.Credentials.Pincode) != patient.RegistrationPin {
		patient.FailedPinCount++

		if patient.FailedPinCount >= maxAttempts {
			patient.RegistrationStatus = datastorer.Blocked
		}

		err = rs.patientStore.PutPatientOnUid(patient)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error storing patient: %s", err)
		}
		return nil, status.Errorf(codes.InvalidArgument, "Invalid pin")
	}

	patient.RegistrationStatus = datastorer.Registered
	patient.RegistrationPin = -1
	err = rs.patientStore.PutPatientOnUid(patient)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error storing patient: %s", err)
	}

	log.Printf("Completed registration of user %+v", patient)

	return &regprotobuf.CompletePatientRegistrationResponse{
		Status: regprotobuf.RegistrationStatus_REGISTRATION_CONFIRMED,
	}, nil
}

func validateRegisterPatientRequest(req *regprotobuf.RegisterPatientRequest) error {
	if req == nil || req.Patient == nil || req.Patient.BSN == "" || req.Patient.FullName == "" || req.Patient.Contact == nil {
		return fmt.Errorf("Missing base fields")
	}
	if req.Patient.Contact.EmailAddress == "" {
		return fmt.Errorf("Missing email")
	}
	return nil
}

func validatePatientRegistrationRequest(req *regprotobuf.CompletePatientRegistrationRequest) error {
	if req == nil || req.PatientUid == "" || req.Credentials == nil || req.Credentials.Pincode <= 0 {
		return fmt.Errorf("Missing credentials")
	}
	return nil
}

func internationalize(phoneNumber string) string {
	if strings.HasPrefix(phoneNumber, "+") {
		return phoneNumber
	}
	return "+" + phoneNumber
}

func patientToInternal(p *regprotobuf.Patient) datastorer.Patient {
	return datastorer.Patient{
		BSN:      p.BSN,
		FullName: p.FullName,
		Address: datastorer.StreetAddress{
			PostalCode: func() string {
				if p.Address != nil {
					return p.Address.PostalCode
				}
				return ""
			}(),
			HouseNumber: func() int {
				if p.Address != nil {
					return int(p.Address.HouseNumber)
				}
				return 0
			}(),
		},
		Contact: datastorer.Contact{
			EmailAddress: func() string {
				if p.Contact != nil {
					return p.Contact.EmailAddress
				}
				return ""
			}(),
		},
	}

}
