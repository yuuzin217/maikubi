export namespace model {
	
	export class Comparison {
	    sourceIndex: number;
	    targetIndex: number;
	    isEqual: boolean;
	    diff: string;
	
	    static createFrom(source: any = {}) {
	        return new Comparison(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceIndex = source["sourceIndex"];
	        this.targetIndex = source["targetIndex"];
	        this.isEqual = source["isEqual"];
	        this.diff = source["diff"];
	    }
	}
	export class DiffLine {
	    lineNumber: number;
	    status: string;
	    text: string;
	    jsonPath: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lineNumber = source["lineNumber"];
	        this.status = source["status"];
	        this.text = source["text"];
	        this.jsonPath = source["jsonPath"];
	    }
	}
	export class Response {
	    status: string;
	    body: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Response(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.body = source["body"];
	        this.error = source["error"];
	    }
	}
	export class DiffResponse {
	    responses: Response[];
	    comparisons: Comparison[];
	    isMatched: boolean;
	    diffLines: DiffLine[];
	
	    static createFrom(source: any = {}) {
	        return new DiffResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.responses = this.convertValues(source["responses"], Response);
	        this.comparisons = this.convertValues(source["comparisons"], Comparison);
	        this.isMatched = source["isMatched"];
	        this.diffLines = this.convertValues(source["diffLines"], DiffLine);
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
	
	export class Targets {
	    production: string;
	    staging: string;
	    baseline: string;
	
	    static createFrom(source: any = {}) {
	        return new Targets(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.production = source["production"];
	        this.staging = source["staging"];
	        this.baseline = source["baseline"];
	    }
	}

}

