export namespace main {
	
	export class ConnectRequest {
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    keyFile: string;
	    trust: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.keyFile = source["keyFile"];
	        this.trust = source["trust"];
	    }
	}
	export class HostKey {
	    fingerprint: string;
	    keyType: string;
	    mismatch: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fingerprint = source["fingerprint"];
	        this.keyType = source["keyType"];
	        this.mismatch = source["mismatch"];
	    }
	}
	export class ConnectResult {
	    id: string;
	    home: string;
	    error: string;
	    hostKey?: HostKey;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.home = source["home"];
	        this.error = source["error"];
	        this.hostKey = this.convertValues(source["hostKey"], HostKey);
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

export namespace remote {
	
	export class Entry {
	    name: string;
	    path: string;
	    dir: boolean;
	    link: boolean;
	    size: number;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.dir = source["dir"];
	        this.link = source["link"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	    }
	}

}

