package javatracker

import (
	"errors"
	"sync"
	"time"
)

type Service struct {
	mu       sync.RWMutex
	project  *Project
	status   IndexStatus
	rebuildM sync.Mutex
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Project() *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.project
}

func (s *Service) Status() IndexStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Service) Build(root string) error {
	s.rebuildM.Lock()
	defer s.rebuildM.Unlock()

	s.mu.Lock()
	s.status = IndexStatus{
		Running:   true,
		Root:      root,
		StartedAt: time.Now(),
	}
	s.mu.Unlock()

	project, err := BuildProject(root)
	finished := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Running = false
	s.status.FinishedAt = finished
	s.status.DurationMS = finished.Sub(s.status.StartedAt).Milliseconds()
	if err != nil {
		s.status.Error = err.Error()
		return err
	}
	s.project = project
	s.status.Root = project.Root
	s.status.ProjectReady = true
	s.status.Stats = project.Stats
	s.status.JavaFiles = project.Stats.JavaFiles
	s.status.FilesSeen = project.Stats.JavaFiles
	s.status.Error = ""
	return nil
}

func (s *Service) Search(query string, limit int) ([]SearchItem, error) {
	project := s.Project()
	if project == nil {
		return nil, errors.New("project not indexed")
	}
	return project.Search(query, limit), nil
}

func (s *Service) Graph(nodeID string, options QueryOptions) (GraphResponse, error) {
	project := s.Project()
	if project == nil {
		return GraphResponse{}, errors.New("project not indexed")
	}
	return project.Graph(nodeID, options)
}

func (s *Service) Details(nodeID string) (NodeDetails, error) {
	project := s.Project()
	if project == nil {
		return NodeDetails{}, errors.New("project not indexed")
	}
	return project.Details(nodeID)
}
