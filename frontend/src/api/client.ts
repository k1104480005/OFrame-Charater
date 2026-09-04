// Typed client over the generated Wails bindings (frontend stays stateless:
// React renders, Go executes/persists — design D11). Every binding resolves to
// typed models; Go errors reject the promise, so callers catch and display.
import {
  AppInfo,
  BaseCharacterAdopt,
  BaseCharacterCandidatesGet,
  BaseCharacterDelete,
  BaseCharacterDescribeImage,
  BaseCharacterFlip,
  BaseCharacterImport,
  BaseCharacterImportCropped,
  BaseCharacterReject,
  BaseCharacterSourceLock,
  CandidateConsistency,
  CandidateDecide,
  CandidateHistory,
  CurrentAssets,
  CurrentPackage,
  DirectionPreview,
  DirectionThumbnail,
  DraftClear,
  DraftGet,
  DraftPut,
  EditDirection,
  EnhanceSettingsGet,
  EnhanceSettingsSet,
  ExportCreate,
  ExportHistory,
  ExportValidate,
  GenerationPlanConfirm,
  GenerationPlanGet,
  GenerationPlanPrepare,
  IdentityAddAnchor,
  IdentityAddAnchorPreset,
  IdentityAnchorPresets,
  IdentityDeleteAnchor,
  IdentityEnhanceDescription,
  CurrentModels,
  IdentityMaterialThumbs,
  IdentityMaterialImage,
  IdentityRemoveMaterial,
  IdentitySetMainReference,
  IdentityGet,
  IdentityImportMaterial,
  IdentityRename,
  IdentitySetCategory,
  IdentitySetCanvas,
  IdentitySetDescription,
  IdentitySetPerfectPixelStandard,
  MotionBatchSummary,
  MotionClearDirection,
  MotionDirectionRawStrip,
  MotionCreate,
  MotionFlipDirection,
  MotionDelete,
  MotionGet,
  MotionList,
  MotionPlaybackTempo,
  MotionRename,
  MotionSetFrameDurations,
  MotionSetGenerationSettings,
  MotionSetLoop,
  MotionSetProviderSettings,
  MotionSetStrategy,
  OperationLog,
  PackageClose,
  PackageCreate,
  PackageDelete,
  PackageOpen,
  PickMaterialFile,
  PresetCatalog,
  ProviderAdd,
  ProviderConfigGet,
  ProviderConfigSave,
  ProviderList,
  ProviderModels,
  ReadImageForPreview,
  ProviderModelsDraft,
  ProviderOptions,
  ProviderPresets,
  ProviderRemove,
  ProviderSetActive,
  ProviderStats,
  ProviderTest,
  ProviderTestDraft,
  ProviderValidate,
  RollbackTo,
  TaskAbandon,
  TaskDelete,
  TaskDeleteFinished,
  TaskGet,
  TaskList,
  TaskResumeAll,
  TaskRetry,
  VideoExtractionConfig,
  WorkspaceEnsureDefault,
  WorkspaceList,
  WorkspaceOpen,
  WorkspacePath,
  WorkspaceMigrate,
  PickWorkspaceDir,
} from "../../wailsjs/go/main/App";
import type { main, assetexport as exportModels } from "../../wailsjs/go/models";
import { EventsOff, EventsOn } from "../../wailsjs/runtime/runtime";

// Re-export the generated model types (all under the `main` namespace).
export type AcceptanceDecisionView = main.AcceptanceDecisionView;
export type AcceptedAssetView = main.AcceptedAssetView;
export type ActionPresetView = main.ActionPresetView;
export type AnchorPresetView = main.AnchorPresetView;
export type AnchorView = main.AnchorView;
export type BaseCharacterCandidateView = main.BaseCharacterCandidateView;
export type AppInfoModel = main.AppInfo;
export type CandidateHistoryView = main.CandidateHistoryView;
export type CandidatePreviewView = main.CandidatePreviewView;
export type CanvasView = main.CanvasView;
export type ConsistencyScoreView = main.ConsistencyScoreView;
export type DirectionView = main.DirectionView;
export type DraftInput = main.DraftInput;
export type DraftView = main.DraftView;
export type EditInstructionView = main.EditInstructionView;
export type EditFrameMetaView = main.EditFrameMetaView;
export type EditResultView = main.EditResultView;
export type FrameView = main.FrameView;
export type GenerationPlanView = main.GenerationPlanView;
export type GenerationRequestView = main.GenerationRequestView;
export type GenerationResultView = main.GenerationResultView;
export type IdentityView = main.IdentityView;
export type MaterialView = main.MaterialView;
export type MaterialThumbView = main.MaterialThumbView;
export type MaterialImageView = main.MaterialImageView;
export type CurrentModelsView = main.CurrentModelsView;
export type MotionBatchSummaryView = main.MotionBatchSummaryView;
export type MotionBatchItemView = main.MotionBatchItemView;
export type MotionBatchCostView = main.MotionBatchCostView;
export type MotionBatchSelectionView = main.MotionBatchSelectionView;
export type MotionView = main.MotionView;
export type OperationLogEntryView = main.OperationLogEntryView;
export type OutboundMaterialView = main.OutboundMaterialView;
export type PackageSummary = main.PackageSummary;
export type PresetCatalogView = main.PresetCatalogView;
export type PreviewFrameView = main.PreviewFrameView;
export type PromptSnapshotView = main.PromptSnapshotView;
export type ProviderConfigView = main.ProviderConfigView;
export type ProviderInfoView = main.ProviderInfoView;
export type ProviderPresetView = main.ProviderPresetView;
export type ProviderOptionView = main.ProviderOptionView;
export type ProviderTestView = main.ProviderTestView;
export type EnhanceSettingsView = main.EnhanceSettingsView;
export type VideoExtractionConfigView = main.VideoExtractionConfigView;
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

// pickWorkspaceDir opens a native directory picker and resolves to the chosen
// absolute path, or "" when the user cancels.
export async function pickWorkspaceDir(): Promise<string> {
  return PickWorkspaceDir("选择工作区目录");
}

// migrateWorkspace copies (move=false) or moves (move=true) the current
// workspace's identity packages into dst and switches the active workspace.
export async function migrateWorkspace(dst: string, move: boolean): Promise<WorkspaceInfo> {
  return WorkspaceMigrate(dst, move);
}

export async function listPackages(): Promise<PackageSummary[]> {
  return WorkspaceList();
}

// --- identity package session (shared core service: core/identity) ---

export async function currentPackage(): Promise<PackageSummary | null> {
  return CurrentPackage();
}

export async function createPackage(name: string, category = ""): Promise<PackageSummary> {
  return PackageCreate(name, category);
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

/** rename an identity package's display name (launch page; directory unchanged) */
export async function renameIdentity(path: string, name: string): Promise<void> {
  return IdentityRename(path, name);
}

/** set an identity package's category (launch page; empty clears it) */
export async function setPackageCategory(path: string, category: string): Promise<void> {
  return IdentitySetCategory(path, category);
}

/** move an identity package to the workspace trash (launch page; recoverable) */
export async function deletePackage(path: string): Promise<void> {
  return PackageDelete(path);
}

export async function saveCanvas(width: number, height: number): Promise<void> {
  return IdentitySetCanvas(width, height);
}

export async function setPerfectPixelStandard(enabled: boolean): Promise<void> {
  return IdentitySetPerfectPixelStandard(enabled);
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

export async function deleteAnchor(id: string): Promise<void> {
  return IdentityDeleteAnchor(id);
}

/** one billed text-model call that expands the description; result needs review */
export async function enhanceDescription(description: string): Promise<string> {
  return IdentityEnhanceDescription(description);
}

/** effective image/text models behind current actions (offline resolution) */
export async function fetchCurrentModels(): Promise<CurrentModelsView> {
  return CurrentModels();
}

/** PNG thumbnails for stored material images (missing/undecodable entries are skipped) */
export async function fetchMaterialThumbs(): Promise<MaterialThumbView[]> {
  return IdentityMaterialThumbs();
}

/** full-resolution stored material image (base64, capped at 20 MB) */
export async function fetchMaterialImage(materialId: string): Promise<MaterialImageView> {
  return IdentityMaterialImage(materialId);
}

/** delete a stored material: manifest entry + file inside the package */
export async function removeMaterial(materialId: string): Promise<void> {
  return IdentityRemoveMaterial(materialId);
}

/** promote an auxiliary reference to main; the current main becomes auxiliary */
export async function setMainReference(materialId: string): Promise<MaterialView> {
  return IdentitySetMainReference(materialId);
}

export async function importMaterial(kind: string, srcPath: string, name: string, role = ""): Promise<MaterialView> {
  return IdentityImportMaterial(kind, srcPath, name, role);
}

/** opens the native file dialog (Go-side) and returns the chosen path */
export async function pickMaterialFile(title: string): Promise<string> {
  return PickMaterialFile(title);
}

// --- base-character creation (character-creation-workflow) ---

/** recorded base-character candidates of the open package, with inline PNG previews */
export async function fetchBaseCharacterCandidates(): Promise<BaseCharacterCandidateView[]> {
  return BaseCharacterCandidatesGet();
}

/** Permanently lock the base-character source for this identity package. */
export async function lockBaseCharacterSource(source: "ai" | "import"): Promise<void> {
  return BaseCharacterSourceLock(source);
}

/** adopt one candidate as the identity's current base character (no external calls) */
export async function adoptBaseCharacter(id: string): Promise<void> {
  return BaseCharacterAdopt(id);
}

/** mark a pending candidate as rejected (弃用) — it can no longer be adopted */
export async function rejectBaseCharacter(id: string): Promise<void> {
  return BaseCharacterReject(id);
}

/** delete a non-adopted candidate record together with its image file (删除候选图) */
export async function deleteBaseCharacter(id: string): Promise<void> {
  return BaseCharacterDelete(id);
}

/** mirror one base-character candidate image horizontally (水平翻转，自逆操作，再点一次翻回） */
export async function flipBaseCharacter(id: string): Promise<void> {
  return BaseCharacterFlip(id);
}

/**
 * record a local sprite image as a PENDING base-character candidate (the
 * import base source; no external calls). The image must match the logical
 * canvas; adopting afterwards is the explicit user decision.
 */
export async function importBaseCharacter(srcPath: string): Promise<BaseCharacterCandidateView> {
  return BaseCharacterImport(srcPath);
}

/**
 * crop the picked image to the given source-pixel rectangle (GUI crop tool,
 * aspect pre-locked to the logical canvas), nearest-resize to the logical
 * canvas and register the result as the pending import draft. No external calls.
 */
export async function importBaseCharacterCropped(
  srcPath: string,
  rect: { x: number; y: number; w: number; h: number },
): Promise<BaseCharacterCandidateView> {
  return BaseCharacterImportCropped(srcPath, rect.x, rect.y, rect.w, rect.h);
}

/** local image file loaded for the webview: raw base64 + pixel dimensions (0 when the Go decoder is unsupported) */
export interface ImageFilePreview {
  width: number;
  height: number;
  mime: string;
  data: string; // base64 of the raw file bytes
}

/** read a local image file for the import crop tool (probe + display in one call) */
export async function readImageForPreview(path: string): Promise<ImageFilePreview> {
  const v = await ReadImageForPreview(path);
  return { width: v.width, height: v.height, mime: v.mime, data: v.data };
}

/**
 * ask the configured prompt-enhancement text model (vision-capable) to
 * describe one base-character candidate image (识图生成描述). One billed text
 * call; the returned text is a suggestion for the description textarea.
 */
export async function describeBaseCharacterImage(candidateId: string): Promise<string> {
  return BaseCharacterDescribeImage(candidateId);
}

// --- unsaved drafts (workbench-ui spec: 草稿在标签切换/任务运行/重启后保留) ---

/** the session package's persisted unsaved draft (zero view when none) */
export async function fetchDraft(): Promise<DraftView> {
  return DraftGet();
}

/** merge a partial draft patch (nil fields stay untouched on the Go side) */
export async function saveDraftPatch(patch: Partial<DraftInput>): Promise<void> {
  return DraftPut(patch);
}

/** drop the session package's entire unsaved draft */
export async function clearDraft(): Promise<void> {
  return DraftClear();
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

/** register a new custom OpenAI-compatible provider (settings preset; id may be empty) */
export async function addProvider(cfg: ProviderConfigView): Promise<ProviderInfoView> {
  return ProviderAdd(cfg);
}

/** delete a custom provider (built-ins are refused by the backend) */
export async function removeProvider(id: string): Promise<void> {
  return ProviderRemove(id);
}

/** live connection test against the provider's /models endpoint */
export async function testProvider(id: string): Promise<ProviderTestView> {
  return ProviderTest(id);
}

/** fetch the model ids exposed by the provider's /models endpoint */
export async function fetchProviderModels(id: string): Promise<string[]> {
  return ProviderModels(id);
}

export async function validateProvider(id: string): Promise<string> {
  return ProviderValidate(id);
}

export async function fetchProviderStats(): Promise<StatsView> {
  return ProviderStats();
}

/** seven FrameBaker quick-preset descriptions (backend-owned defaults; task 4.1/5.1) */
export async function fetchProviderPresets(): Promise<ProviderPresetView[]> {
  return ProviderPresets();
}

/** connection test against UNSAVED form values — persists nothing (task 4.2/5.3) */
export async function testProviderDraft(cfg: ProviderConfigView): Promise<ProviderTestView> {
  return ProviderTestDraft(cfg);
}

/** model discovery against UNSAVED form values — persists nothing (task 4.3/5.3) */
export async function fetchProviderModelsDraft(cfg: ProviderConfigView): Promise<string[]> {
  return ProviderModelsDraft(cfg);
}

/** capability-filtered provider/model choices ("image" | "video" | "text"; task 4.4) */
export async function fetchProviderOptions(capability: string): Promise<ProviderOptionView[]> {
  return ProviderOptions(capability);
}

/** prompt-enhancement association (provider + text model; task 5.5) */
export async function fetchEnhanceSettings(): Promise<EnhanceSettingsView> {
  return EnhanceSettingsGet();
}

export async function setEnhanceSettings(providerId: string, model: string): Promise<void> {
  return EnhanceSettingsSet(providerId, model);
}

/** read-only video-model config of a provider; Supported stays false until the video pipeline lands (task 6.2) */
export async function fetchVideoExtractionConfig(id: string): Promise<VideoExtractionConfigView> {
  return VideoExtractionConfig(id);
}

// --- PerfectPixel presets (四个风格预设 + 动作预设) ---

export async function fetchPresetCatalog(): Promise<PresetCatalogView> {
  return PresetCatalog();
}

// --- generation confirmation (生成确认) ---

export async function prepareGeneration(req: GenerationRequestView): Promise<GenerationPlanView> {
  return GenerationPlanPrepare(req);
}

/** re-fetch a prepared plan by id（任务卡切标签后恢复确认弹窗用） */
export async function fetchGenerationPlan(planId: string): Promise<GenerationPlanView> {
  return GenerationPlanGet(planId);
}

export async function confirmGeneration(planId: string, accept: boolean): Promise<GenerationResultView> {
  return GenerationPlanConfirm(planId, accept);
}

// --- task drawer (typed registry; full queue engine lands with tasks 6.x) ---

export async function fetchTasks(): Promise<TaskSummary[]> {
  return TaskList();
}

export async function deleteTask(id: string): Promise<void> {
  return TaskDelete(id);
}

export async function deleteFinishedTasks(): Promise<number> {
  return TaskDeleteFinished();
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

export async function deleteMotion(id: string): Promise<void> {
  return MotionDelete(id);
}

export async function setMotionStrategy(id: string, count: number, mirror: boolean): Promise<MotionView> {
  return MotionSetStrategy(id, count, mirror);
}

export async function setMotionGenerationSettings(
  id: string,
  actionPresetID: string,
  actionDescription: string,
  frameCount: number,
): Promise<MotionView> {
  return MotionSetGenerationSettings(id, actionPresetID, actionDescription, frameCount);
}

export async function setMotionProviderSettings(id: string, providerID: string, model: string): Promise<MotionView> {
  return MotionSetProviderSettings(id, providerID, model);
}

export async function setMotionLoop(id: string, loop: boolean): Promise<MotionView> {
  return MotionSetLoop(id, loop);
}

export async function renameMotion(id: string, name: string): Promise<MotionView> {
  return MotionRename(id, name);
}

/** 删除一格动画（九宫格右键"删除"）：该方向回到未生成状态，可重新点亮生成 */
export async function clearMotionDirection(id: string, direction: string): Promise<MotionView> {
  return MotionClearDirection(id, direction);
}

/** 方向动画的原始条带图（九宫格右键"预览原图"：大模型返回、未切分的 base64 PNG） */
export async function fetchDirectionRawStrip(id: string, direction: string): Promise<string> {
  return MotionDirectionRawStrip(id, direction);
}

/** 水平翻转一格动画（九宫格右键）：镜像对同步翻转，自逆操作，返回确认文案 */
export async function flipMotionDirection(id: string, direction: string): Promise<string> {
  return MotionFlipDirection(id, direction);
}

export async function setMotionFrameDurations(id: string, direction: string, durationsMs: number[]): Promise<MotionView> {
  return MotionSetFrameDurations(id, direction, durationsMs);
}

/** 批量操作区统计：按各动作卡的勾选方向统计未生成格子/调用/费用（offline，不建 plan） */
export async function fetchBatchSummary(selections: MotionBatchSelectionView[]): Promise<MotionBatchSummaryView> {
  return MotionBatchSummary(selections);
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

/** 方向动画第 1 帧的 base64 PNG（九宫格缩略图） */
export async function fetchDirectionThumbnail(motionId: string, direction: string): Promise<string> {
  return DirectionThumbnail(motionId, direction);
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
