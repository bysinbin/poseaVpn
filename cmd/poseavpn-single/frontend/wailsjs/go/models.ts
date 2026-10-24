export namespace guiapp {
	
	export class Credential {
	    username: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new Credential(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class ImportResult {
	    imported: string[];
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.errors = source["errors"];
	    }
	}
	export class ProfileInfo {
	    name: string;
	    path: string;
	    addresses: string[];
	    endpoint: string;
	    allowedIps: string[];
	    dns: string[];
	    transport: string;
	    proto: string;
	    needsAuth: boolean;
	    active: boolean;
	    external: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.addresses = source["addresses"];
	        this.endpoint = source["endpoint"];
	        this.allowedIps = source["allowedIps"];
	        this.dns = source["dns"];
	        this.transport = source["transport"];
	        this.proto = source["proto"];
	        this.needsAuth = source["needsAuth"];
	        this.active = source["active"];
	        this.external = source["external"];
	        this.error = source["error"];
	    }
	}
	export class State {
	    mode: string;
	    version: string;
	    service: string;
	    tunnels: tunnel.StatusInfo[];
	    profiles: ProfileInfo[];
	    lastConfig?: ProfileInfo;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.version = source["version"];
	        this.service = source["service"];
	        this.tunnels = this.convertValues(source["tunnels"], tunnel.StatusInfo);
	        this.profiles = this.convertValues(source["profiles"], ProfileInfo);
	        this.lastConfig = this.convertValues(source["lastConfig"], ProfileInfo);
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

export namespace tunnel {
	
	export class StatusInfo {
	    active: boolean;
	    tunnel_name?: string;
	    addresses?: string[];
	    endpoints?: string[];
	    allowed_ips?: string[];
	    transport?: string;
	    connected_at?: number;
	    last_handshake?: number;
	    rx_bytes?: number;
	    tx_bytes?: number;
	    proto?: string;
	    dns_proxies?: string[];
	    mappings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new StatusInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.tunnel_name = source["tunnel_name"];
	        this.addresses = source["addresses"];
	        this.endpoints = source["endpoints"];
	        this.allowed_ips = source["allowed_ips"];
	        this.transport = source["transport"];
	        this.connected_at = source["connected_at"];
	        this.last_handshake = source["last_handshake"];
	        this.rx_bytes = source["rx_bytes"];
	        this.tx_bytes = source["tx_bytes"];
	        this.proto = source["proto"];
	        this.dns_proxies = source["dns_proxies"];
	        this.mappings = source["mappings"];
	    }
	}

}

