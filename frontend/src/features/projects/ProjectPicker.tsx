/**
 * The project chooser.
 *
 * It is shown when no project is open and when the user asks to switch.
 * Every project is a workspace directory and a Git repository on disk;
 * this panel only lists the records.
 */

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../bindings';
import type { Project } from '../../bindings';

interface ProjectPickerProps {
  onOpen: (projectId: string) => void;
  notify: (message: string, kind?: 'info' | 'error' | 'success') => void;
}

/**
 * The project chooser.
 */
export function ProjectPicker({ onOpen, notify }: ProjectPickerProps) {
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const [pendingDelete, setPendingDelete] = useState('');

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => api.listProjects(),
  });

  const createMutation = useMutation({
    mutationFn: (projectName: string) => api.createProject(projectName),
    onSuccess: (project) => {
      setName('');
      void queryClient.invalidateQueries({ queryKey: ['projects'] });
      onOpen(project.id);
    },
    onError: (error: Error) => notify(error.message, 'error'),
  });

  const deleteMutation = useMutation({
    mutationFn: (projectId: string) => api.deleteProject(projectId),
    onSuccess: () => {
      setPendingDelete('');
      void queryClient.invalidateQueries({ queryKey: ['projects'] });
      notify('The project was deleted.');
    },
    onError: (error: Error) => notify(error.message, 'error'),
  });

  const projects: Project[] = projectsQuery.data ?? [];

  return (
    <div className="project-picker">
      <div className="project-picker-card">
        <h1 className="project-picker-title">Your writing projects</h1>
        <p className="text-dim">
          Each project is a Markdown workspace with its own local Git history. Nothing leaves this
          machine.
        </p>

        <form
          className="project-create"
          onSubmit={(event) => {
            event.preventDefault();
            const trimmed = name.trim();
            if (trimmed) {
              createMutation.mutate(trimmed);
            }
          }}
        >
          <input
            type="text"
            value={name}
            placeholder="New project name"
            aria-label="New project name"
            onChange={(event) => setName(event.target.value)}
          />
          <button
            type="submit"
            className="btn btn-primary"
            disabled={!name.trim() || createMutation.isPending}
          >
            Create
          </button>
        </form>

        {projectsQuery.error ? (
          <p className="text-dim">The projects could not be loaded.</p>
        ) : (
          <ul className="project-list">
            {projects.map((project) => (
              <li key={project.id} className="project-row">
                <button type="button" className="project-open" onClick={() => onOpen(project.id)}>
                  <strong>{project.name}</strong>
                  <span className="text-dim">
                    {project.word_count} words · updated {new Date(project.updated).toLocaleDateString()}
                  </span>
                </button>
                {pendingDelete === project.id ? (
                  <button
                    type="button"
                    className="btn btn-sm btn-danger"
                    disabled={deleteMutation.isPending}
                    onClick={() => deleteMutation.mutate(project.id)}
                  >
                    Confirm delete
                  </button>
                ) : (
                  <button
                    type="button"
                    className="binder-row-action"
                    title="Delete the project"
                    onClick={() => setPendingDelete(project.id)}
                  >
                    ×
                  </button>
                )}
              </li>
            ))}
            {projects.length === 0 && !projectsQuery.isLoading && (
              <li className="text-dim">No projects yet. Create one to start writing.</li>
            )}
          </ul>
        )}
      </div>
    </div>
  );
}
