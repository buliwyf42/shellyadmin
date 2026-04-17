package services

import (
	"errors"
	"fmt"
	"strings"

	"shellyadmin/internal/models"
)

func (s *AppService) ListCredentials() ([]models.Credential, error) {
	return s.db.ListCredentials()
}

func (s *AppService) SaveCredential(c models.Credential) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Username = strings.TrimSpace(c.Username)
	if c.Name == "" {
		return errors.New("credential name is required")
	}
	if c.Username == "" {
		return errors.New("credential username is required")
	}
	if strings.TrimSpace(c.Password) == "" && strings.TrimSpace(c.HA1) == "" {
		return errors.New("credential requires password or ha1")
	}
	c.Tags = sanitizeTags(c.Tags)
	return s.db.SaveCredential(c)
}

func (s *AppService) DeleteCredential(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("credential name is required")
	}
	templates, err := s.db.ListTemplateNames()
	if err != nil {
		return err
	}
	for _, templateName := range templates {
		_, credentialRef, err := s.db.GetTemplate(templateName)
		if err != nil {
			continue
		}
		if credentialRef == name {
			return fmt.Errorf("credential is referenced by template %q", templateName)
		}
	}
	return s.db.DeleteCredential(name)
}

func (s *AppService) ListCredentialGroups() ([]models.CredentialGroup, error) {
	return s.db.ListCredentialGroups()
}

func (s *AppService) SaveCredentialGroup(group models.CredentialGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		return errors.New("group name is required")
	}
	if strings.TrimSpace(group.Password) == "" && strings.TrimSpace(group.HA1) == "" {
		return errors.New("group requires password or ha1")
	}
	group.Tags = sanitizeTags(group.Tags)
	if err := s.db.SaveCredentialGroup(group); err != nil {
		return err
	}
	return s.db.SaveCredential(models.Credential{
		Name:     group.Name,
		Username: "admin",
		Password: group.Password,
		HA1:      group.HA1,
		Tags:     group.Tags,
	})
}

func (s *AppService) DeleteCredentialGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("group name is required")
	}
	if err := s.DeleteCredential(name); err != nil {
		return err
	}
	return s.db.DeleteCredentialGroup(name)
}

func (s *AppService) ListCredentialGroupAssignments() (map[string]string, error) {
	assignments, err := s.db.ListDeviceCredentialGroupAssignments()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, assignment := range assignments {
		out[assignment.MAC] = assignment.GroupName
	}
	return out, nil
}

func (s *AppService) SaveCredentialGroupAssignments(macs []string, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	cleaned := make([]string, 0, len(macs))
	seen := map[string]bool{}
	for _, mac := range macs {
		trimmed := strings.TrimSpace(mac)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) == 0 {
		return errors.New("macs required")
	}
	if groupName != "" {
		groups, err := s.db.ListCredentialGroups()
		if err != nil {
			return err
		}
		found := false
		for _, group := range groups {
			if group.Name == groupName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("group %q not found", groupName)
		}
	}
	return s.db.SaveDeviceCredentialGroupAssignments(cleaned, groupName)
}
