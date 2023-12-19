import { Component } from '@angular/core';
import { UserService } from 'src/app/services';
import { User } from 'src/app/models/user';
import { HttpClient } from '@angular/common/http';
import { ElementRef, ViewChild } from '@angular/core';
import { FormBuilder, FormGroup, Validators ,AbstractControl} from '@angular/forms';





@Component({
  selector: 'app-edit-profile',
  templateUrl: './edit-profile.component.html',
  styleUrls: ['./edit-profile.component.css']
})
export class EditProfileComponent {
  currentUser: User = {} as User; 
  currentPassword = '';
  newPassword = '';
  confirmNewPassword = '';
  currentUsername ='';
  currentEmail= "";
  address : FormGroup = new FormGroup({});
  new : any
  currentRole='';
  changeInfoForm: FormGroup = new FormGroup({});

  notification = { msgType: '', msgBody: '' };
  notification2 = { msgType: '', msgBody: '' };

  constructor(private userService: UserService, private http: HttpClient,private formBuilder: FormBuilder
    ) {
    this.currentEmail=this.userService.currentUser.user.email
    this.currentUsername=this.userService.currentUser.user.username
    this.currentRole=this.userService.currentUser.user.userRole
    this.changeInfoForm = this.formBuilder.group({
   
      email: ['', [Validators.required, Validators.email, Validators.minLength(6), Validators.maxLength(64)]],
      name: ['', [Validators.required, Validators.minLength(1), Validators.maxLength(64)]],
      lastname: ['', [Validators.required, Validators.minLength(1), Validators.maxLength(64)]],
            street: ['', Validators.required],
            city: ['', Validators.required],
            country: ['', Validators.required],
      age: ['', [Validators.maxLength(3)]],
      gender: [''],
    });
   
  }

  changePassword() {
    if (this.newPassword !== this.confirmNewPassword) {
      this.notification = { msgType: 'error', msgBody: `New passwords do not match.` };
      return;
    }
    const user = this.userService.currentUser;
    console.log("1",this.currentPassword, this.newPassword, this.confirmNewPassword)


    this.userService.changePassword(this.currentPassword, this.newPassword, this.confirmNewPassword)
      .subscribe(
       () => {
        this.notification = { msgType: 'success', msgBody: `Password changed successfully.` };
        this.resetForm();
        },
        (_:any) => {
          this.notification = { msgType: 'error', msgBody: `Failed to change password.` };
        }
      );
      
  }
  saveChanges(){
    
    this.new = {}
    this.new.username= this.currentUsername
    this.new.email= this.currentEmail
    this.new.name= this.changeInfoForm.get('name')?.value
    this.new.lastname= this.changeInfoForm.get('lastname')?.value
    this.new.address= {}
    this.new.address.street= this.changeInfoForm.get('street')?.value
    this.new.address.city= this.changeInfoForm.get('city')?.value
    this.new.address.country= this.changeInfoForm.get('country')?.value
    this.new.age= this.changeInfoForm.get('age')?.value
    this.new.gender= this.changeInfoForm.get('gender')?.value
    this.new.userRole= this.currentRole

    console.log(this.new)


    this.http.post('https://localhost:8084/api/profile/updateUser', this.new).subscribe(
      () => {
        this.notification2 = { msgType: 'success', msgBody: 'Profile updated successfully.' };
      },
      (error) => {
        console.error('Error updating profile', error);
        this.notification2 = { msgType: 'error', msgBody: 'Failed to update profile.' };
      }
    );
  }
  resetForm() {
    this.currentPassword = '';
    this.newPassword = '';
    this.confirmNewPassword = '';
    this.currentUsername = '';
  }
}
