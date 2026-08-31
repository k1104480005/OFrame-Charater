export namespace assetexport {
	
	export class Anchor {
	    id?: string;
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new Anchor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class Rect {
	    x: number;
	    y: number;
	    w: number;
	    h: number;
	
	    static createFrom(source: any = {}) {
	        return new Rect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.w = source["w"];
	        this.h = source["h"];
	    }
	}
	export class FrameManifest {
	    index: number;
	    file: string;
	    rect: Rect;
	    durationMs: number;
	    anchors?: Anchor[];
	
	    static createFrom(source: any = {}) {
	        return new FrameManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.file = source["file"];
	        this.rect = this.convertValues(source["rect"], Rect);
	        this.durationMs = source["durationMs"];
	        this.anchors = this.convertValues(source["anchors"], Anchor);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AnimationManifest {
	    motionId: string;
	    direction: string;
	    candidateId?: string;
	    frames: FrameManifest[];
	
	    static createFrom(source: any = {}) {
	        return new AnimationManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.motionId = source["motionId"];
	        this.direction = source["direction"];
	        this.candidateId = source["candidateId"];
	        this.frames = this.convertValues(source["frames"], FrameManifest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class HistoryRecord {
	    target: string;
	    identityVersion: string;
	    outputDir: string;
	    result: string;
	    error?: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.identityVersion = source["identityVersion"];
	        this.outputDir = source["outputDir"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Manifest {
	    formatVersion: number;
	    target: string;
	    identityVersion: string;
	    generatedAt: string;
	    spriteSheet: string;
	    cellWidth: number;
	    cellHeight: number;
	    animations: AnimationManifest[];
	
	    static createFrom(source: any = {}) {
	        return new Manifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.formatVersion = source["formatVersion"];
	        this.target = source["target"];
	        this.identityVersion = source["identityVersion"];
	        this.generatedAt = source["generatedAt"];
	        this.spriteSheet = source["spriteSheet"];
	        this.cellWidth = source["cellWidth"];
	        this.cellHeight = source["cellHeight"];
	        this.animations = this.convertValues(source["animations"], AnimationManifest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Result {
	    outputDir: string;
	    target: string;
	    manifest: Manifest;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputDir = source["outputDir"];
	        this.target = source["target"];
	        this.manifest = this.convertValues(source["manifest"], Manifest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class AcceptanceDecisionView {
	    candidateId: string;
	    decision: string;
	    note: string;
	    threshold: number;
	
	    static createFrom(source: any = {}) {
	        return new AcceptanceDecisionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.candidateId = source["candidateId"];
	        this.decision = source["decision"];
	        this.note = source["note"];
	        this.threshold = source["threshold"];
	    }
	}
	export class AcceptedAssetView {
	    motionId?: string;
	    direction: string;
	    candidateId: string;
	    acceptedAt: string;
	    frameCount: number;
	
	    static createFrom(source: any = {}) {
	        return new AcceptedAssetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.motionId = source["motionId"];
	        this.direction = source["direction"];
	        this.candidateId = source["candidateId"];
	        this.acceptedAt = source["acceptedAt"];
	        this.frameCount = source["frameCount"];
	    }
	}
	export class ActionPresetView {
	    id: string;
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionPresetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class AnchorPointView {
	    name: string;
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new AnchorPointView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class AnchorPresetView {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new AnchorPresetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class AnchorView {
	    id: string;
	    name: string;
	    preset: string;
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new AnchorView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.preset = source["preset"];
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class AppInfo {
	    name: string;
	    version: string;
	    go: string;
	    formatVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.go = source["go"];
	        this.formatVersion = source["formatVersion"];
	    }
	}
	export class BaseCharacterCandidateView {
	    id: string;
	    imagePath: string;
	    png: string;
	    provider?: string;
	    model?: string;
	    status: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new BaseCharacterCandidateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.imagePath = source["imagePath"];
	        this.png = source["png"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class CandidateHistoryView {
	    id: string;
	    motionId?: string;
	    direction?: string;
	    createdAt: string;
	    status: string;
	    overall: number;
	    acceptanceNote?: string;
	    regenerationOf?: string;
	
	    static createFrom(source: any = {}) {
	        return new CandidateHistoryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.motionId = source["motionId"];
	        this.direction = source["direction"];
	        this.createdAt = source["createdAt"];
	        this.status = source["status"];
	        this.overall = source["overall"];
	        this.acceptanceNote = source["acceptanceNote"];
	        this.regenerationOf = source["regenerationOf"];
	    }
	}
	export class PreviewFrameView {
	    index: number;
	    png: string;
	    durationMs: number;
	    anchors?: pipeline.AnchorPoint[];
	
	    static createFrom(source: any = {}) {
	        return new PreviewFrameView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.png = source["png"];
	        this.durationMs = source["durationMs"];
	        this.anchors = this.convertValues(source["anchors"], pipeline.AnchorPoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CandidatePreviewView {
	    motionId: string;
	    direction: string;
	    origin: string;
	    source?: string;
	    canvasWidth: number;
	    canvasHeight: number;
	    frames: PreviewFrameView[];
	
	    static createFrom(source: any = {}) {
	        return new CandidatePreviewView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.motionId = source["motionId"];
	        this.direction = source["direction"];
	        this.origin = source["origin"];
	        this.source = source["source"];
	        this.canvasWidth = source["canvasWidth"];
	        this.canvasHeight = source["canvasHeight"];
	        this.frames = this.convertValues(source["frames"], PreviewFrameView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CanvasView {
	    unitWidth: number;
	    unitHeight: number;
	
	    static createFrom(source: any = {}) {
	        return new CanvasView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.unitWidth = source["unitWidth"];
	        this.unitHeight = source["unitHeight"];
	    }
	}
	export class ConsistencyScoreView {
	    score: number;
	    source: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConsistencyScoreView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.score = source["score"];
	        this.source = source["source"];
	        this.detail = source["detail"];
	    }
	}
	export class CurrentModelsView {
	    providerId: string;
	    providerName: string;
	    imageModel?: string;
	    imageModels?: string[];
	    enhanceProviderId?: string;
	    enhanceModel?: string;
	    enhanceSupported: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CurrentModelsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.providerName = source["providerName"];
	        this.imageModel = source["imageModel"];
	        this.imageModels = source["imageModels"];
	        this.enhanceProviderId = source["enhanceProviderId"];
	        this.enhanceModel = source["enhanceModel"];
	        this.enhanceSupported = source["enhanceSupported"];
	    }
	}
	export class DirectionResultView {
	    direction: string;
	    attempts: number;
	    bytes: number;
	    model: string;
	    candidateId?: string;
	
	    static createFrom(source: any = {}) {
	        return new DirectionResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.direction = source["direction"];
	        this.attempts = source["attempts"];
	        this.bytes = source["bytes"];
	        this.model = source["model"];
	        this.candidateId = source["candidateId"];
	    }
	}
	export class FrameView {
	    index: number;
	    assetRef?: string;
	    durationMs: number;
	    anchors?: AnchorPointView[];
	
	    static createFrom(source: any = {}) {
	        return new FrameView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.assetRef = source["assetRef"];
	        this.durationMs = source["durationMs"];
	        this.anchors = this.convertValues(source["anchors"], AnchorPointView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DirectionView {
	    direction: string;
	    origin: string;
	    source?: string;
	    frames: FrameView[];
	
	    static createFrom(source: any = {}) {
	        return new DirectionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.direction = source["direction"];
	        this.origin = source["origin"];
	        this.source = source["source"];
	        this.frames = this.convertValues(source["frames"], FrameView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DraftInput {
	    description?: string;
	    motionName?: string;
	    motionCount?: number;
	    motionMirror?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DraftInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.motionName = source["motionName"];
	        this.motionCount = source["motionCount"];
	        this.motionMirror = source["motionMirror"];
	    }
	}
	export class DraftView {
	    description: string;
	    motionName: string;
	    motionCount: number;
	    motionMirror?: boolean;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DraftView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.motionName = source["motionName"];
	        this.motionCount = source["motionCount"];
	        this.motionMirror = source["motionMirror"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class EditFrameMetaView {
	    durationMs: number;
	    anchorX: number;
	    anchorY: number;
	
	    static createFrom(source: any = {}) {
	        return new EditFrameMetaView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.durationMs = source["durationMs"];
	        this.anchorX = source["anchorX"];
	        this.anchorY = source["anchorY"];
	    }
	}
	export class EditInstructionView {
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
	    frameMeta?: EditFrameMetaView;
	
	    static createFrom(source: any = {}) {
	        return new EditInstructionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.frameIndex = source["frameIndex"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.r = source["r"];
	        this.g = source["g"];
	        this.b = source["b"];
	        this.a = source["a"];
	        this.durationMs = source["durationMs"];
	        this.deltaX = source["deltaX"];
	        this.deltaY = source["deltaY"];
	        this.order = source["order"];
	        this.framePng = source["framePng"];
	        this.frameMeta = this.convertValues(source["frameMeta"], EditFrameMetaView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EditResultView {
	    motionId: string;
	    direction: string;
	    frameCount: number;
	    durationsMs: number[];
	    logSeq: number;
	
	    static createFrom(source: any = {}) {
	        return new EditResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.motionId = source["motionId"];
	        this.direction = source["direction"];
	        this.frameCount = source["frameCount"];
	        this.durationsMs = source["durationsMs"];
	        this.logSeq = source["logSeq"];
	    }
	}
	export class EnhanceSettingsView {
	    providerId: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new EnhanceSettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	    }
	}
	
	export class PromptSnapshotView {
	    stylePresetId: string;
	    actionPresetId: string;
	    description: string;
	    referenceMaterialIds: string[];
	    canvasWidth: number;
	    canvasHeight: number;
	    frameCount: number;
	    directions: number;
	    prompt: string;
	    builtAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PromptSnapshotView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stylePresetId = source["stylePresetId"];
	        this.actionPresetId = source["actionPresetId"];
	        this.description = source["description"];
	        this.referenceMaterialIds = source["referenceMaterialIds"];
	        this.canvasWidth = source["canvasWidth"];
	        this.canvasHeight = source["canvasHeight"];
	        this.frameCount = source["frameCount"];
	        this.directions = source["directions"];
	        this.prompt = source["prompt"];
	        this.builtAt = source["builtAt"];
	    }
	}
	export class OutboundMaterialView {
	    materialId: string;
	    kind: string;
	    role: string;
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new OutboundMaterialView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.materialId = source["materialId"];
	        this.kind = source["kind"];
	        this.role = source["role"];
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class GenerationPlanView {
	    id: string;
	    kind: string;
	    motionId?: string;
	    providerId: string;
	    providerType?: string;
	    model: string;
	    capability: string;
	    directions: number;
	    basicDirections: number;
	    mirroredDirections: number;
	    basicLabels?: string[];
	    mirroredLabels?: string[];
	    expectedCalls: number;
	    maxAttemptsPerDirection: number;
	    maxTotalAttempts: number;
	    outboundMaterials: OutboundMaterialView[];
	    prompt: PromptSnapshotView;
	    costPerCall: number;
	    currency: string;
	    expectedCost: number;
	    maxCost: number;
	    status: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerationPlanView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.motionId = source["motionId"];
	        this.providerId = source["providerId"];
	        this.providerType = source["providerType"];
	        this.model = source["model"];
	        this.capability = source["capability"];
	        this.directions = source["directions"];
	        this.basicDirections = source["basicDirections"];
	        this.mirroredDirections = source["mirroredDirections"];
	        this.basicLabels = source["basicLabels"];
	        this.mirroredLabels = source["mirroredLabels"];
	        this.expectedCalls = source["expectedCalls"];
	        this.maxAttemptsPerDirection = source["maxAttemptsPerDirection"];
	        this.maxTotalAttempts = source["maxTotalAttempts"];
	        this.outboundMaterials = this.convertValues(source["outboundMaterials"], OutboundMaterialView);
	        this.prompt = this.convertValues(source["prompt"], PromptSnapshotView);
	        this.costPerCall = source["costPerCall"];
	        this.currency = source["currency"];
	        this.expectedCost = source["expectedCost"];
	        this.maxCost = source["maxCost"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GenerationRequestView {
	    packagePath: string;
	    baseCharacter?: boolean;
	    motionId?: string;
	    providerId: string;
	    model: string;
	    directions: number;
	    disableMirror?: boolean;
	    stylePresetId: string;
	    styleCustom?: string;
	    description?: string;
	    actionPresetId: string;
	    frameCount: number;
	    maxAttemptsPerDirection: number;
	    replaceDirections?: string[];
	    regenerateOf?: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerationRequestView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.packagePath = source["packagePath"];
	        this.baseCharacter = source["baseCharacter"];
	        this.motionId = source["motionId"];
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	        this.directions = source["directions"];
	        this.disableMirror = source["disableMirror"];
	        this.stylePresetId = source["stylePresetId"];
	        this.styleCustom = source["styleCustom"];
	        this.description = source["description"];
	        this.actionPresetId = source["actionPresetId"];
	        this.frameCount = source["frameCount"];
	        this.maxAttemptsPerDirection = source["maxAttemptsPerDirection"];
	        this.replaceDirections = source["replaceDirections"];
	        this.regenerateOf = source["regenerateOf"];
	    }
	}
	export class GenerationResultView {
	    planId: string;
	    accepted: boolean;
	    status: string;
	    callsMade: number;
	    attempts: number;
	    results: DirectionResultView[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerationResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.planId = source["planId"];
	        this.accepted = source["accepted"];
	        this.status = source["status"];
	        this.callsMade = source["callsMade"];
	        this.attempts = source["attempts"];
	        this.results = this.convertValues(source["results"], DirectionResultView);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VersionView {
	    id: string;
	    createdAt: string;
	    reason: string;
	    immutable: boolean;
	    assetsRef: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = source["createdAt"];
	        this.reason = source["reason"];
	        this.immutable = source["immutable"];
	        this.assetsRef = source["assetsRef"];
	    }
	}
	export class MaterialView {
	    id: string;
	    kind: string;
	    role: string;
	    name: string;
	    path: string;
	    addedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new MaterialView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.role = source["role"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.addedAt = source["addedAt"];
	    }
	}
	export class IdentityView {
	    name: string;
	    id: string;
	    description: string;
	    entryKind: string;
	    baseCharacterId?: string;
	    baseCharacterSource?: string;
	    perfectPixelStandard: boolean;
	    canvas?: CanvasView;
	    anchors: AnchorView[];
	    materials: MaterialView[];
	    versions: VersionView[];
	    currentVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new IdentityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.id = source["id"];
	        this.description = source["description"];
	        this.entryKind = source["entryKind"];
	        this.baseCharacterId = source["baseCharacterId"];
	        this.baseCharacterSource = source["baseCharacterSource"];
	        this.perfectPixelStandard = source["perfectPixelStandard"];
	        this.canvas = this.convertValues(source["canvas"], CanvasView);
	        this.anchors = this.convertValues(source["anchors"], AnchorView);
	        this.materials = this.convertValues(source["materials"], MaterialView);
	        this.versions = this.convertValues(source["versions"], VersionView);
	        this.currentVersion = source["currentVersion"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MaterialImageView {
	    materialId: string;
	    mime: string;
	    data: string;
	
	    static createFrom(source: any = {}) {
	        return new MaterialImageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.materialId = source["materialId"];
	        this.mime = source["mime"];
	        this.data = source["data"];
	    }
	}
	export class MaterialThumbView {
	    materialId: string;
	    png: string;
	
	    static createFrom(source: any = {}) {
	        return new MaterialThumbView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.materialId = source["materialId"];
	        this.png = source["png"];
	    }
	}
	
	export class StrategyView {
	    count: number;
	    mirror: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StrategyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.mirror = source["mirror"];
	    }
	}
	export class MotionView {
	    id: string;
	    name: string;
	    strategy: StrategyView;
	    directions: DirectionView[];
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new MotionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.strategy = this.convertValues(source["strategy"], StrategyView);
	        this.directions = this.convertValues(source["directions"], DirectionView);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OperationLogEntryView {
	    seq: number;
	    at: string;
	    action: string;
	    payload?: any;
	
	    static createFrom(source: any = {}) {
	        return new OperationLogEntryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.at = source["at"];
	        this.action = source["action"];
	        this.payload = source["payload"];
	    }
	}
	
	export class PackageSummary {
	    name: string;
	    path: string;
	    category?: string;
	    formatVersion: number;
	    currentVersion: string;
	    createdAt: string;
	    updatedAt: string;
	    baseCharacterSource?: string;
	
	    static createFrom(source: any = {}) {
	        return new PackageSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.category = source["category"];
	        this.formatVersion = source["formatVersion"];
	        this.currentVersion = source["currentVersion"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.baseCharacterSource = source["baseCharacterSource"];
	    }
	}
	export class StylePresetView {
	    id: string;
	    name: string;
	    description: string;
	    negativePrompt: string;
	
	    static createFrom(source: any = {}) {
	        return new StylePresetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.negativePrompt = source["negativePrompt"];
	    }
	}
	export class PresetCatalogView {
	    styles: StylePresetView[];
	    actions: ActionPresetView[];
	
	    static createFrom(source: any = {}) {
	        return new PresetCatalogView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.styles = this.convertValues(source["styles"], StylePresetView);
	        this.actions = this.convertValues(source["actions"], ActionPresetView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class ProviderConfigView {
	    providerId: string;
	    type: string;
	    name: string;
	    apiKey: string;
	    model: string;
	    videoModel: string;
	    textModel: string;
	    imageModels?: string[];
	    videoModels?: string[];
	    textModels?: string[];
	    baseUrl: string;
	    defaultSize?: string;
	    maxAttempts: number;
	    timeoutSec: number;
	    pricePerCall: number;
	    cliCommand?: string;
	    cliPromptArg?: string;
	    cliOutputArg?: string;
	    cliModelArg?: string;
	    cliRefImageArg?: string;
	    cliExtraArgs?: string[];
	    cliTemplate?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	        this.videoModel = source["videoModel"];
	        this.textModel = source["textModel"];
	        this.imageModels = source["imageModels"];
	        this.videoModels = source["videoModels"];
	        this.textModels = source["textModels"];
	        this.baseUrl = source["baseUrl"];
	        this.defaultSize = source["defaultSize"];
	        this.maxAttempts = source["maxAttempts"];
	        this.timeoutSec = source["timeoutSec"];
	        this.pricePerCall = source["pricePerCall"];
	        this.cliCommand = source["cliCommand"];
	        this.cliPromptArg = source["cliPromptArg"];
	        this.cliOutputArg = source["cliOutputArg"];
	        this.cliModelArg = source["cliModelArg"];
	        this.cliRefImageArg = source["cliRefImageArg"];
	        this.cliExtraArgs = source["cliExtraArgs"];
	        this.cliTemplate = source["cliTemplate"];
	    }
	}
	export class ProviderInfoView {
	    id: string;
	    type: string;
	    name: string;
	    builtin: boolean;
	    active: boolean;
	    image: boolean;
	    video: boolean;
	    text: boolean;
	    imageModel: string;
	    videoModel: string;
	    textModel: string;
	    imageModels: string[];
	    videoModels: string[];
	    textModels: string[];
	    baseUrl: string;
	    hasApiKey: boolean;
	    keySource: string;
	    maxAttempts: number;
	    pricePerCall: number;
	    currency: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfoView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.builtin = source["builtin"];
	        this.active = source["active"];
	        this.image = source["image"];
	        this.video = source["video"];
	        this.text = source["text"];
	        this.imageModel = source["imageModel"];
	        this.videoModel = source["videoModel"];
	        this.textModel = source["textModel"];
	        this.imageModels = source["imageModels"];
	        this.videoModels = source["videoModels"];
	        this.textModels = source["textModels"];
	        this.baseUrl = source["baseUrl"];
	        this.hasApiKey = source["hasApiKey"];
	        this.keySource = source["keySource"];
	        this.maxAttempts = source["maxAttempts"];
	        this.pricePerCall = source["pricePerCall"];
	        this.currency = source["currency"];
	    }
	}
	export class ProviderOptionView {
	    id: string;
	    name: string;
	    type: string;
	    models: string[];
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderOptionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.models = source["models"];
	        this.reason = source["reason"];
	    }
	}
	export class ProviderPresetView {
	    key: string;
	    name: string;
	    description: string;
	    type: string;
	    baseUrl: string;
	    image: boolean;
	    video: boolean;
	    text: boolean;
	    imageModels: string[];
	    videoModels: string[];
	    textModels: string[];
	    cliPromptArg?: string;
	    cliOutputArg?: string;
	    cliModelArg?: string;
	    cliRefImageArg?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderPresetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.type = source["type"];
	        this.baseUrl = source["baseUrl"];
	        this.image = source["image"];
	        this.video = source["video"];
	        this.text = source["text"];
	        this.imageModels = source["imageModels"];
	        this.videoModels = source["videoModels"];
	        this.textModels = source["textModels"];
	        this.cliPromptArg = source["cliPromptArg"];
	        this.cliOutputArg = source["cliOutputArg"];
	        this.cliModelArg = source["cliModelArg"];
	        this.cliRefImageArg = source["cliRefImageArg"];
	    }
	}
	export class ProviderTestView {
	    ok: boolean;
	    latencyMs?: number;
	    models?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderTestView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.latencyMs = source["latencyMs"];
	        this.models = source["models"];
	        this.error = source["error"];
	    }
	}
	export class StatView {
	    providerId: string;
	    model: string;
	    callCount: number;
	    estimatedCost: number;
	    currency: string;
	    lastCallAt: string;
	
	    static createFrom(source: any = {}) {
	        return new StatView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	        this.callCount = source["callCount"];
	        this.estimatedCost = source["estimatedCost"];
	        this.currency = source["currency"];
	        this.lastCallAt = source["lastCallAt"];
	    }
	}
	export class StatsView {
	    totalCalls: number;
	    items: StatView[];
	
	    static createFrom(source: any = {}) {
	        return new StatsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalCalls = source["totalCalls"];
	        this.items = this.convertValues(source["items"], StatView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class TaskSummary {
	    id: string;
	    kind: string;
	    status: string;
	    progress: number;
	    error: string;
	    retryCount: number;
	    createdAt: string;
	    updatedAt: string;
	    live: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TaskSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.error = source["error"];
	        this.retryCount = source["retryCount"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.live = source["live"];
	    }
	}
	
	export class VideoExtractionConfigView {
	    providerId: string;
	    type: string;
	    videoModels: string[];
	    supported: boolean;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoExtractionConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.type = source["type"];
	        this.videoModels = source["videoModels"];
	        this.supported = source["supported"];
	        this.reason = source["reason"];
	    }
	}
	export class WorkspaceInfo {
	    path: string;
	    packageCount: number;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.packageCount = source["packageCount"];
	    }
	}

}

export namespace pipeline {
	
	export class AnchorPoint {
	    Name: string;
	    X: number;
	    Y: number;
	
	    static createFrom(source: any = {}) {
	        return new AnchorPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.X = source["X"];
	        this.Y = source["Y"];
	    }
	}

}

