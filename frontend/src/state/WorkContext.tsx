// Shared workbench object context (workbench-context-preview spec): the
// current motion/direction/candidate is SESSION-level state — every view
// (make/acceptance/edit/export) reads and writes the same selection, so one
// choice updates all views and switching views never loses the current object.
// Preview controls (play/grid/anchors) ride the same context so they persist
// across tab switches; previewNonce lets any view (generation finished, edit
// applied, rollback) tell the preview owners to reload their artifacts.
// Opening a DIFFERENT package invalidates every object reference.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { useSession } from "./SessionContext";

export interface PreviewControls {
  playing: boolean;
  showGrid: boolean;
  showAnchors: boolean;
}

export interface WorkState {
  motionId: string;
  direction: string;
  candidateId: string;
  preview: PreviewControls;
  /** increments whenever the current candidate's artifacts changed — views reload their preview */
  previewNonce: number;
  /** switch the current motion (resets direction + candidate) */
  selectMotion: (motionId: string) => void;
  /** switch the current direction of the current motion (resets candidate) */
  selectDirection: (direction: string) => void;
  /** deep-link focus: point the context at a candidate, optionally re-pointing motion/direction */
  focusCandidate: (candidateId: string, motionId?: string, direction?: string) => void;
  setPreview: (patch: Partial<PreviewControls>) => void;
  /** signal "current artifacts changed" without changing the selection */
  bumpPreview: () => void;
}

const WorkContext = createContext<WorkState | null>(null);

export function WorkbenchProvider({ children }: { children: ReactNode }) {
  const { pkg } = useSession();
  const [motionId, setMotionId] = useState("");
  const [direction, setDirection] = useState("");
  const [candidateId, setCandidateId] = useState("");
  const [preview, setPreviewState] = useState<PreviewControls>({ playing: false, showGrid: true, showAnchors: true });
  const [previewNonce, setPreviewNonce] = useState(0);

  // A different open package invalidates every object reference; view
  // switches and re-emits of the SAME package keep the selection.
  const pkgPath = pkg?.path ?? "";
  useEffect(() => {
    setMotionId("");
    setDirection("");
    setCandidateId("");
    setPreviewState({ playing: false, showGrid: true, showAnchors: true });
  }, [pkgPath]);

  const selectMotion = useCallback((id: string) => {
    setMotionId(id);
    setDirection("");
    setCandidateId("");
  }, []);

  const selectDirection = useCallback((dir: string) => {
    setDirection(dir);
    setCandidateId("");
  }, []);

  const focusCandidate = useCallback((cand: string, mid?: string, dir?: string) => {
    setCandidateId(cand);
    if (mid !== undefined) setMotionId(mid);
    if (dir !== undefined) setDirection(dir);
    setPreviewNonce((n) => n + 1);
  }, []);

  const setPreview = useCallback((patch: Partial<PreviewControls>) => {
    setPreviewState((p) => ({ ...p, ...patch }));
  }, []);

  const bumpPreview = useCallback(() => setPreviewNonce((n) => n + 1), []);

  const value = useMemo<WorkState>(
    () => ({ motionId, direction, candidateId, preview, previewNonce, selectMotion, selectDirection, focusCandidate, setPreview, bumpPreview }),
    [motionId, direction, candidateId, preview, previewNonce, selectMotion, selectDirection, focusCandidate, setPreview, bumpPreview],
  );

  return <WorkContext.Provider value={value}>{children}</WorkContext.Provider>;
}

export function useWork(): WorkState {
  const ctx = useContext(WorkContext);
  if (!ctx) throw new Error("useWork must be used inside WorkbenchProvider");
  return ctx;
}
