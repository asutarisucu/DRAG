export namespace archive {
	
	export class Entry {
	    path: string;
	    size: number;
	    dir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.dir = source["dir"];
	    }
	}

}

export namespace main {
	
	export class ArchiveInfo {
	    id: string;
	    entries: archive.Entry[];
	
	    static createFrom(source: any = {}) {
	        return new ArchiveInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.entries = this.convertValues(source["entries"], archive.Entry);
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
	    sudo: boolean;
	
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
	        this.sudo = source["sudo"];
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
	export class Pairing {
	    url: string;
	    expires: number;
	    path: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new Pairing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.expires = source["expires"];
	        this.path = source["path"];
	        this.size = source["size"];
	    }
	}
	export class PhoneSettings {
	    lan: boolean;
	    port: number;
	    public: boolean;
	    publicPort: number;
	
	    static createFrom(source: any = {}) {
	        return new PhoneSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lan = source["lan"];
	        this.port = source["port"];
	        this.public = source["public"];
	        this.publicPort = source["publicPort"];
	    }
	}
	export class PhoneState {
	    settings: PhoneSettings;
	    status: phone.Status;
	    public: phone.Status;
	
	    static createFrom(source: any = {}) {
	        return new PhoneState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], PhoneSettings);
	        this.status = this.convertValues(source["status"], phone.Status);
	        this.public = this.convertValues(source["public"], phone.Status);
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
	export class ServerInfo {
	    id: string;
	    name: string;
	    user: string;
	    host: string;
	    home: string;
	    info?: remote.Info;
	
	    static createFrom(source: any = {}) {
	        return new ServerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.user = source["user"];
	        this.host = source["host"];
	        this.home = source["home"];
	        this.info = this.convertValues(source["info"], remote.Info);
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
	export class ShellInfo {
	    id: string;
	    owner: string;
	    fg: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new ShellInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.owner = source["owner"];
	        this.fg = source["fg"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}

}

export namespace phone {
	
	export class Device {
	    id: string;
	    name: string;
	    hash: string;
	    route: string;
	    // Go type: time
	    created: any;
	    // Go type: time
	    lastSeen: any;
	    lastIP: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.hash = source["hash"];
	        this.route = source["route"];
	        this.created = this.convertValues(source["created"], null);
	        this.lastSeen = this.convertValues(source["lastSeen"], null);
	        this.lastIP = source["lastIP"];
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
	export class Login {
	    // Go type: time
	    time: any;
	    kind: string;
	    device: string;
	    ip: string;
	    route: string;
	    what?: string;
	
	    static createFrom(source: any = {}) {
	        return new Login(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.kind = source["kind"];
	        this.device = source["device"];
	        this.ip = source["ip"];
	        this.route = source["route"];
	        this.what = source["what"];
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
	export class Status {
	    running: boolean;
	    url: string;
	    error: string;
	    certUntil: number;
	    pending: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.url = source["url"];
	        this.error = source["error"];
	        this.certUntil = source["certUntil"];
	        this.pending = source["pending"];
	    }
	}

}

export namespace remote {
	
	export class Conn {
	    remote: string;
	    port: number;
	    proc: string;
	
	    static createFrom(source: any = {}) {
	        return new Conn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.remote = source["remote"];
	        this.port = source["port"];
	        this.proc = source["proc"];
	    }
	}
	export class Container {
	    ID: string;
	    Names: string;
	    Image: string;
	    State: string;
	    Status: string;
	    Ports: string;
	    Labels: string;
	    Mounts: string;
	    project: string;
	    compose: string;
	
	    static createFrom(source: any = {}) {
	        return new Container(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Names = source["Names"];
	        this.Image = source["Image"];
	        this.State = source["State"];
	        this.Status = source["Status"];
	        this.Ports = source["Ports"];
	        this.Labels = source["Labels"];
	        this.Mounts = source["Mounts"];
	        this.project = source["project"];
	        this.compose = source["compose"];
	    }
	}
	export class DirSize {
	    path: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new DirSize(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	    }
	}
	export class Volume {
	    Name: string;
	    Driver: string;
	    Mountpoint: string;
	
	    static createFrom(source: any = {}) {
	        return new Volume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Driver = source["Driver"];
	        this.Mountpoint = source["Mountpoint"];
	    }
	}
	export class Image {
	    ID: string;
	    Repository: string;
	    Tag: string;
	    Size: string;
	    CreatedSince: string;
	
	    static createFrom(source: any = {}) {
	        return new Image(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Repository = source["Repository"];
	        this.Tag = source["Tag"];
	        this.Size = source["Size"];
	        this.CreatedSince = source["CreatedSince"];
	    }
	}
	export class DockerInfo {
	    containers: Container[];
	    images: Image[];
	    volumes: Volume[];
	
	    static createFrom(source: any = {}) {
	        return new DockerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containers = this.convertValues(source["containers"], Container);
	        this.images = this.convertValues(source["images"], Image);
	        this.volumes = this.convertValues(source["volumes"], Volume);
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
	export class DockerMount {
	    Type: string;
	    Name: string;
	    Source: string;
	    Destination: string;
	
	    static createFrom(source: any = {}) {
	        return new DockerMount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.Name = source["Name"];
	        this.Source = source["Source"];
	        this.Destination = source["Destination"];
	    }
	}
	export class DockerStat {
	    Name: string;
	    CPUPerc: string;
	    MemUsage: string;
	
	    static createFrom(source: any = {}) {
	        return new DockerStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.CPUPerc = source["CPUPerc"];
	        this.MemUsage = source["MemUsage"];
	    }
	}
	export class Entry {
	    name: string;
	    path: string;
	    dir: boolean;
	    link: boolean;
	    size: number;
	    modTime: number;
	    locked: boolean;
	
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
	        this.locked = source["locked"];
	    }
	}
	export class Iface {
	    name: string;
	    addrs: string[];
	
	    static createFrom(source: any = {}) {
	        return new Iface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addrs = source["addrs"];
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
	export class Listen {
	    port: number;
	    addr: string;
	    proc: string;
	
	    static createFrom(source: any = {}) {
	        return new Listen(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.addr = source["addr"];
	        this.proc = source["proc"];
	    }
	}
	export class LogChunk {
	    text: string;
	    start: number;
	    end: number;
	
	    static createFrom(source: any = {}) {
	        return new LogChunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.start = source["start"];
	        this.end = source["end"];
	    }
	}
	export class Mount {
	    path: string;
	    dev: string;
	    size: number;
	    used: number;
	
	    static createFrom(source: any = {}) {
	        return new Mount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.dev = source["dev"];
	        this.size = source["size"];
	        this.used = source["used"];
	    }
	}
	export class NetInfo {
	    ifaces: Iface[];
	    conns: Conn[];
	    listen: Listen[];
	
	    static createFrom(source: any = {}) {
	        return new NetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ifaces = this.convertValues(source["ifaces"], Iface);
	        this.conns = this.convertValues(source["conns"], Conn);
	        this.listen = this.convertValues(source["listen"], Listen);
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
	export class Proc {
	    pid: number;
	    user: string;
	    name: string;
	    cmd: string;
	    cpu: number;
	    rss: number;
	
	    static createFrom(source: any = {}) {
	        return new Proc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.user = source["user"];
	        this.name = source["name"];
	        this.cmd = source["cmd"];
	        this.cpu = source["cpu"];
	        this.rss = source["rss"];
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
	export class Service {
	    name: string;
	    desc: string;
	    active: string;
	    sub: string;
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.desc = source["desc"];
	        this.active = source["active"];
	        this.sub = source["sub"];
	    }
	}
	export class Session {
	    kind: string;
	    name: string;
	    attached: boolean;
	    desc: string;
	    lastUsed: number;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.attached = source["attached"];
	        this.desc = source["desc"];
	        this.lastUsed = source["lastUsed"];
	    }
	}
	export class TextFile {
	    text: string;
	    modTime: number;
	    locked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TextFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.modTime = source["modTime"];
	        this.locked = source["locked"];
	    }
	}
	export class TrashEntry {
	    name: string;
	    from: string;
	    deleted: number;
	    dir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrashEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.from = source["from"];
	        this.deleted = source["deleted"];
	        this.dir = source["dir"];
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
	
	export class Point {
	    t: number;
	    cpu: number;
	    mem: number;
	    disk: number;
	    rx: number;
	    tx: number;
	
	    static createFrom(source: any = {}) {
	        return new Point(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.t = source["t"];
	        this.cpu = source["cpu"];
	        this.mem = source["mem"];
	        this.disk = source["disk"];
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	    }
	}
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

