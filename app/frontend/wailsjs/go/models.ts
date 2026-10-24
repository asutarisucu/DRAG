export namespace main {
	
	export class ConnectRequest {
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    auth: string;
	    password: string;
	    keyFile: string;
	    trust: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.auth = source["auth"];
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
	    info?: remote.Info;
	    error: string;
	    hostKey?: HostKey;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.home = source["home"];
	        this.info = this.convertValues(source["info"], remote.Info);
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
	
	export class LocalEntry {
	    path: string;
	    name: string;
	    dir: boolean;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new LocalEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.dir = source["dir"];
	        this.size = source["size"];
	    }
	}
	export class Replay {
	    data: string;
	    end: number;
	
	    static createFrom(source: any = {}) {
	        return new Replay(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = source["data"];
	        this.end = source["end"];
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
	export class Info {
	    os: string;
	    arch: string;
	    distro: string;
	    systemd: boolean;
	    docker: boolean;
	    screen: boolean;
	    tmux: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.distro = source["distro"];
	        this.systemd = source["systemd"];
	        this.docker = source["docker"];
	        this.screen = source["screen"];
	        this.tmux = source["tmux"];
	    }
	}
	export class Props {
	    type: string;
	    size: number;
	    mode: string;
	    owner: string;
	    group: string;
	    modTime: number;
	    target: string;
	
	    static createFrom(source: any = {}) {
	        return new Props(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.owner = source["owner"];
	        this.group = source["group"];
	        this.modTime = source["modTime"];
	        this.target = source["target"];
	    }
	}
	export class Session {
	    kind: string;
	    name: string;
	    attached: boolean;
	    desc: string;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.attached = source["attached"];
	        this.desc = source["desc"];
	    }
	}
	export class TrashItem {
	    name: string;
	    from: string;
	
	    static createFrom(source: any = {}) {
	        return new TrashItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.from = source["from"];
	    }
	}

}

export namespace store {
	
	export class Server {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    auth: string;
	    keyFile: string;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.auth = source["auth"];
	        this.keyFile = source["keyFile"];
	    }
	}

}

