// Typed client over the generated Wails bindings (frontend stays stateless:
// React renders, Go executes/persists — design D11). Every binding resolves to
// typed models; Go errors reject the promise, so callers catch and display.
import {
  AppInfo,
  CandidateConsistency,
  CandidateDecide,
  CandidateHistory,
  CurrentAssets,
  CurrentPackage,
  DirectionPreview,
  EditDirection,
  ExportCreate,
  ExportHistory,
  ExportValidate,
  GenerationPlanConfirm,
  GenerationPlanPrepare,
  IdentityAddAnchor,
  IdentityAddAnchorPreset,
  IdentityAnchorPresets,
  IdentityGet,
  IdentityImportMaterial,
  IdentitySetCanvas,
  IdentitySetDescription,
  MotionCreate,
  MotionGet,
  MotionList,
  MotionPlaybackTempo,
  MotionSetFrameDurations,
  MotionSetStrategy,
  OperationLog,
  PackageClose,
  PackageCreate,
  PackageOpen,
  PickMaterialFile,
  PresetCatalog,
  ProviderConfigGet,
  ProviderConfigSave,
  ProviderList,
  ProviderSetActive,
  ProviderStats,
  ProviderValidate,
  RollbackTo,
  TaskAbandon,
  TaskGet,
  TaskList,
  TaskResumeAll,
  TaskRetry,
  WorkspaceEnsureDefault,
  WorkspaceList,
  WorkspaceOpen,
  WorkspacePath,
} from "../../wailsjs/go/main/App";
import type { main, assetexport as exportModels } from "../../wailsjs/go/models";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";

// Re-export the generated model types (all under the `main` namespace).
export type AcceptanceDecisionView = main.AcceptanceDecisionView;
export type AcceptedAssetView = main.AcceptedAssetView;
export type ActionPresetView = main.ActionPresetView;
export type AnchorPresetView = main.AnchorPresetView;
export type AnchorView = main.AnchorView;
export type AppInfoModel = main.AppInfo;
export type CandidateHistoryView = main.CandidateHistoryView;
export type CandidatePreviewView = main.CandidatePreviewView;
export type CanvasView = main.CanvasView;
export type ConsistencyScoreView = main.ConsistencyScoreView;
export type DirectionView = main.DirectionView;
export type EditInstructionView = main.EditInstructionView;
export type EditFrameMetaView = main.EditFrameMetaView;
export type EditResultView = main.EditResultView;
export type FrameView = main.FrameView;
export type GenerationPlanView = main.GenerationPlanView;
export type GenerationRequestView = main.GenerationRequestView;
export type GenerationResultView = main.GenerationResultView;
export type IdentityView = main.IdentityView;
export type MaterialView = main.MaterialView;
export type MotionView = main.MotionView;
export type OperationLogEntryView = main.OperationLogEntryView;
export type OutboundMaterialView = main.OutboundMaterialView;
export type PackageSummary = main.PackageSummary;
export type PresetCatalogView = main.PresetCatalogView;
export type PreviewFrameView = main.PreviewFrameView;
export type PromptSnapshotView = main.PromptSnapshotView;
export type ProviderConfigView = main.ProviderConfigView;
export type ProviderInfoView = main.ProviderInfoView;
export type StatView = main.StatView;
export type StatsView = main.StatsView;
export type StrategyView = main.StrategyView;
export type StylePresetView = main.StylePresetView;
export type TaskSummary = main.TaskSummary;
export type VersionView = main.VersionView;
export type WorkspaceInfo = main.WorkspaceInfo;

// --- export capability (tasks 11.x). The Go package core/assetexport maps to
// the generated `assetexport` namespace (the old name `export` collided with a
// TS reserved word), aliased here as `exportModels`. ---
export type ExportResult = exportModels.Result;
export type ExportManifest = exportModels.Manifest;
export type ExportAnimationManifest = exportModels.AnimationManifest;
export type ExportFrameManifest = exportModels.FrameManifest;
export type ExportAnchor = exportModels.Anchor;
export type ExportRect = exportModels.Rect;
export type ExportHistoryRecord = exportModels.HistoryRecord;

// --- events (runtime events 基础) ---

export const EventSessionChanged = "session:changed";
export const EventTasksChanged = "task:changed";

export function onSessionChanged(cb: (pkg: PackageSummary | null) => void): () => void {
  return EventsOn(EventSessionChanged, (pkg: PackageSummary | null) => cb(pkg ?? null));
}

export function onTasksChanged(cb: (tasks: TaskSummary[]) => void): () => void {
  return EventsOn(EventTasksChanged, (tasks: TaskSummary[]) => cb(tasks ?? []));
}

export function offEvents(name: string, ...rest: string[]): void {
  EventsOff(name, ...rest);
}

// --- app info ---

export async function fetchAppInfo(): Promise<AppInfoModel> {
  return AppInfo();
}

// --- workspace (shared core service: core/workspace) ---

export async function ensureDefaultWorkspace(): Promise<WorkspaceInfo> {
  return WorkspaceEnsureDefault();
}

export async function openWorkspace(path: string): Promise<WorkspaceInfo> {
  return WorkspaceOpen(path);
}

export async function currentWorkspacePath(): Promise<string> {
  return WorkspacePath();
}

export async function listPackages(): Promise<PackageSummary[]> {
  return WorkspaceList();
}

// --- identity package session (shared core service: core/identity) ---

export async function currentPackage(): Promise<PackageSummary | null> {
  return CurrentPackage();
}

export async function createPackage(name: string): Promise<PackageSummary> {
  return PackageCreate(name);
}

export async function openPackage(path: string): Promise<PackageSummary> {
  return PackageOpen(path);
}

export async function closePackage(): Promise<void> {
  return PackageClose();
}

// --- identity sub-page ---

export async function fetchIdentity(): Promise<IdentityView> {
  return IdentityGet();
}

export async function saveDescription(text: string): Promise<void> {
  return IdentitySetDescription(text);
}

export async function saveCanvas(width: number, height: number): Promise<void> {
  return IdentitySetCanvas(width, height);
}

export async function fetchAnchorPresets(): Promise<AnchorPresetView[]> {
  return IdentityAnchorPresets();
}

export async function addAnchorPreset(presetId: string, name: string): Promise<AnchorView> {
  return IdentityAddAnchorPreset(presetId, name);
}

export async function addAnchor(name: string, presetId: string, x: number, y: number): Promise<AnchorView> {
  return IdentityAddAnchor(name, presetId, x, y);
}

export async function importMaterial(kind: string, srcPath: string, name: string, role = ""): Promise<MaterialView> {
  return IdentityImportMaterial(kind, srcPath, name, role);
}

/** opens the native file dialog (Go-side) and returns the chosen path */
export async function pickMaterialFile(title: string): Promise<string> {
  return PickMaterialFile(title);
}

// --- provider configuration & validation (模型/密钥配置与验证, shared service) ---

export async function fetchProviders(): Promise<ProviderInfoView[]> {
  return ProviderList();
}

export async function fetchProviderConfig(id: string): Promise<ProviderConfigView> {
  return ProviderConfigGet(id);
}

export async function saveProviderConfig(id: string, cfg: ProviderConfigView): Promise<void> {
  return ProviderConfigSave(id, cfg);
}

export async function setActiveProvider(id: string): Promise<void> {
  return ProviderSetActive(id);
}

export async function validateProvider(id: string): Promise<string> {
  return ProviderValidate(id);
}

export async function fetchProviderStats(): Promise<StatsView> {
  return ProviderStats();
}

// --- PerfectPixel presets (四个风格预设 + 动作预设) ---

export async function fetchPresetCatalog(): Promise<PresetCatalogView> {
  return PresetCatalog();
}

// --- generation confirmation (生成确认) ---

export async function prepareGeneration(req: GenerationRequestView): Promise<GenerationPlanView> {
  return GenerationPlanPrepare(req);
}

export async function confirmGeneration(planId: string, accept: boolean): Promise<GenerationResultView> {
  return GenerationPlanConfirm(planId, accept);
}

// --- task drawer (typed registry; full queue engine lands with tasks 6.x) ---

export async function fetchTasks(): Promise<TaskSummary[]> {
  return TaskList();
}

export async function retryTask(id: string): Promise<void> {
  return TaskRetry(id);
}

export async function abandonTask(id: string): Promise<void> {
  return TaskAbandon(id);
}

// --- recoverable task queue (tasks 6.1–6.5) ---

export async function fetchTask(id: string): Promise<TaskSummary> {
  return TaskGet(id);
}

/** one-click resume of unfinished tasks after an interruption (task 6.3) */
export async function resumeAllTasks(): Promise<number> {
  return TaskResumeAll();
}

// --- motion capability (阶段 5: 动作/方向集/帧序列) ---

export async function createMotion(name: string, count: number, mirror: boolean): Promise<MotionView> {
  return MotionCreate(name, count, mirror);
}

export async function fetchMotions(): Promise<MotionView[]> {
  return MotionList();
}

export async function fetchMotion(id: string): Promise<MotionView> {
  return MotionGet(id);
}

export async function setMotionStrategy(id: string, count: number, mirror: boolean): Promise<MotionView> {
  return MotionSetStrategy(id, count, mirror);
}

export async function setMotionFrameDurations(id: string, direction: string, durationsMs: number[]): Promise<MotionView> {
  return MotionSetFrameDurations(id, direction, durationsMs);
}

export async function fetchMotionTempo(id: string, direction: string): Promise<number[]> {
  return MotionPlaybackTempo(id, direction);
}

// --- quality acceptance (tasks 8.2–8.4, 9.2–9.4) ---

export async function fetchCandidateHistory(): Promise<CandidateHistoryView[]> {
  return CandidateHistory();
}

export async function decideCandidate(candidateId: string, confirm: boolean, note: string): Promise<AcceptanceDecisionView> {
  return CandidateDecide(candidateId, confirm, note);
}

export async function fetchOperationLog(): Promise<OperationLogEntryView[]> {
  return OperationLog();
}

export async function rollbackTo(seq: number): Promise<OperationLogEntryView[]> {
  return RollbackTo(seq);
}

export async function fetchCurrentAssets(): Promise<AcceptedAssetView[]> {
  return CurrentAssets();
}

export async function fetchConsistencyScore(useAI: boolean): Promise<ConsistencyScoreView> {
  return CandidateConsistency(useAI);
}

// --- PixelPerfect preview (task 5.5) ---

export async function fetchDirectionPreview(motionId: string, direction: string): Promise<CandidatePreviewView> {
  return DirectionPreview(motionId, direction);
}

// --- lightweight editing (阶段 7: 可回放编辑指令, 任务 7.1–7.5) ---

/** plain edit instruction input (mapped to the generated model by the wrapper) */
export interface EditInstructionInput {
  kind: string;
  frameIndex?: number;
  x?: number;
  y?: number;
  width?: number;
  height?: number;
  r?: number;
  g?: number;
  b?: number;
  a?: number;
  durationMs?: number;
  deltaX?: number;
  deltaY?: number;
  order?: number[];
  framePng?: string;
  frameMeta?: { durationMs?: number; anchorX?: number; anchorY?: number };
}

/** apply replayable edit instructions to a motion direction's animation assets */
export async function editDirection(
  motionId: string,
  direction: string,
  instructions: EditInstructionInput[],
): Promise<EditResultView> {
  // The generated model class carries a `convertValues` method; the Wails
  // bridge serializes plain JSON, so a structural cast is sufficient here.
  return EditDirection(motionId, direction, instructions as unknown as EditInstructionView[]);
}

// --- export capability (tasks 11.1–11.4) ---

/** build a validated export package from accepted assets of the selected version */
export async function exportPackage(outputDir: string, target: string, versionID: string): Promise<ExportResult> {
  return ExportCreate(outputDir, target, versionID);
}

/** re-validate a previously generated export package on disk */
export async function validateExport(outputDir: string): Promise<void> {
  return ExportValidate(outputDir);
}

/** read the export operation history of the session package */
export async function fetchExportHistory(): Promise<ExportHistoryRecord[]> {
  return ExportHistory();
}
