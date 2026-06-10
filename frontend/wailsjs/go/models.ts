export namespace main {
	
	export class ActivationResult {
	    success: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ActivationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	    }
	}
	export class ConversionResult {
	    success: boolean;
	    message: string;
	    tableCount: number;
	    excelPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ConversionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.tableCount = source["tableCount"];
	        this.excelPath = source["excelPath"];
	    }
	}
	export class FileDialogResult {
	    success: boolean;
	    filePath: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDialogResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.filePath = source["filePath"];
	        this.message = source["message"];
	    }
	}
	export class LicenseCheckResult {
	    valid: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LicenseCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.message = source["message"];
	    }
	}

}

