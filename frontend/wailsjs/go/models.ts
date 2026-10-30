export namespace env {
	
	export class CacheTargetItem {
	    id: string;
	    label: string;
	    path: string;
	    exists: boolean;
	    sizeBytes: number;
	    sizeHuman: string;
	    defaultTick: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CacheTargetItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.path = source["path"];
	        this.exists = source["exists"];
	        this.sizeBytes = source["sizeBytes"];
	        this.sizeHuman = source["sizeHuman"];
	        this.defaultTick = source["defaultTick"];
	    }
	}
	export class OrphanedPathEntry {
	    rawPath: string;
	    expandedPath: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new OrphanedPathEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rawPath = source["rawPath"];
	        this.expandedPath = source["expandedPath"];
	        this.reason = source["reason"];
	    }
	}
	export class CleanupInspection {
	    installDir: string;
	    managedPathEntries: string[];
	    pubCacheVarValue: string;
	    orphanedPaths: OrphanedPathEntry[];
	    diskTargets: CacheTargetItem[];
	
	    static createFrom(source: any = {}) {
	        return new CleanupInspection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installDir = source["installDir"];
	        this.managedPathEntries = source["managedPathEntries"];
	        this.pubCacheVarValue = source["pubCacheVarValue"];
	        this.orphanedPaths = this.convertValues(source["orphanedPaths"], OrphanedPathEntry);
	        this.diskTargets = this.convertValues(source["diskTargets"], CacheTargetItem);
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
	export class CleanupRequest {
	    installDir: string;
	    removeManagedEnv: boolean;
	    orphanPathsToRemove: string[];
	    deletePortableDir: boolean;
	    cacheIdsToDelete: string[];
	
	    static createFrom(source: any = {}) {
	        return new CleanupRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installDir = source["installDir"];
	        this.removeManagedEnv = source["removeManagedEnv"];
	        this.orphanPathsToRemove = source["orphanPathsToRemove"];
	        this.deletePortableDir = source["deletePortableDir"];
	        this.cacheIdsToDelete = source["cacheIdsToDelete"];
	    }
	}
	export class CleanupResult {
	    removedPaths: string[];
	    pubCacheRemoved: boolean;
	    deletedDirectories: string[];
	    terminatedProcs: number;
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CleanupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.removedPaths = source["removedPaths"];
	        this.pubCacheRemoved = source["pubCacheRemoved"];
	        this.deletedDirectories = source["deletedDirectories"];
	        this.terminatedProcs = source["terminatedProcs"];
	        this.warnings = source["warnings"];
	    }
	}
	
	export class PreflightReport {
	    status: string;
	    statusLabel: string;
	    installDir: string;
	    detectedMode: string;
	    flutterVersion: string;
	    dartVersion: string;
	    gitAvailable: boolean;
	    gitSource: string;
	    gitPath: string;
	    pathAligned: boolean;
	    pubCacheAligned: boolean;
	    sdkExistsOnDisk: boolean;
	    movedFromDir?: string;
	    expectedSdkBin: string;
	    expectedPubCache: string;
	    expectedPubCacheBin: string;
	    expectedMinGitCmd?: string;
	
	    static createFrom(source: any = {}) {
	        return new PreflightReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.statusLabel = source["statusLabel"];
	        this.installDir = source["installDir"];
	        this.detectedMode = source["detectedMode"];
	        this.flutterVersion = source["flutterVersion"];
	        this.dartVersion = source["dartVersion"];
	        this.gitAvailable = source["gitAvailable"];
	        this.gitSource = source["gitSource"];
	        this.gitPath = source["gitPath"];
	        this.pathAligned = source["pathAligned"];
	        this.pubCacheAligned = source["pubCacheAligned"];
	        this.sdkExistsOnDisk = source["sdkExistsOnDisk"];
	        this.movedFromDir = source["movedFromDir"];
	        this.expectedSdkBin = source["expectedSdkBin"];
	        this.expectedPubCache = source["expectedPubCache"];
	        this.expectedPubCacheBin = source["expectedPubCacheBin"];
	        this.expectedMinGitCmd = source["expectedMinGitCmd"];
	    }
	}

}

export namespace sdk {
	
	export class ProvisionRequest {
	    mode: string;
	    sourceType: string;
	    version: string;
	    dartVersion: string;
	    downloadUrl: string;
	    expectedSha256: string;
	    sha256Url: string;
	    localZipPath: string;
	    installDir: string;
	    autoInstallMinGit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProvisionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.sourceType = source["sourceType"];
	        this.version = source["version"];
	        this.dartVersion = source["dartVersion"];
	        this.downloadUrl = source["downloadUrl"];
	        this.expectedSha256 = source["expectedSha256"];
	        this.sha256Url = source["sha256Url"];
	        this.localZipPath = source["localZipPath"];
	        this.installDir = source["installDir"];
	        this.autoInstallMinGit = source["autoInstallMinGit"];
	    }
	}
	export class ProvisionResult {
	    mode: string;
	    version: string;
	    dartVersion: string;
	    installDir: string;
	    minGitInstalled: boolean;
	    registeredPaths: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProvisionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.version = source["version"];
	        this.dartVersion = source["dartVersion"];
	        this.installDir = source["installDir"];
	        this.minGitInstalled = source["minGitInstalled"];
	        this.registeredPaths = source["registeredPaths"];
	    }
	}
	export class ReleaseOption {
	    version: string;
	    dartVersion: string;
	    channel: string;
	    releaseDate: string;
	    downloadUrl: string;
	    sha256: string;
	    sha256Url?: string;
	    isDefault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.dartVersion = source["dartVersion"];
	        this.channel = source["channel"];
	        this.releaseDate = source["releaseDate"];
	        this.downloadUrl = source["downloadUrl"];
	        this.sha256 = source["sha256"];
	        this.sha256Url = source["sha256Url"];
	        this.isDefault = source["isDefault"];
	    }
	}
	export class ReleaseCatalog {
	    defaultFlutterVersion: string;
	    defaultDartVersion: string;
	    flutterReleases: ReleaseOption[];
	    dartReleases: ReleaseOption[];
	
	    static createFrom(source: any = {}) {
	        return new ReleaseCatalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultFlutterVersion = source["defaultFlutterVersion"];
	        this.defaultDartVersion = source["defaultDartVersion"];
	        this.flutterReleases = this.convertValues(source["flutterReleases"], ReleaseOption);
	        this.dartReleases = this.convertValues(source["dartReleases"], ReleaseOption);
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

